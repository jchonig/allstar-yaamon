// Package ami provides an AllStar/Asterisk Manager Interface (AMI) client.
package ami

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a parsed AMI message: a block of Key: Value headers terminated by a blank line.
type Event struct {
	Headers map[string]string
}

func (e Event) Get(key string) string { return e.Headers[key] }
func (e Event) Type() string          { return e.Headers["Event"] }
func (e Event) IsResponse() bool      { _, ok := e.Headers["Response"]; return ok }

// Client manages a single persistent AMI TCP connection with automatic reconnect.
type Client struct {
	nodeID int64
	host   string
	port   int
	user   string
	pass   string

	events    chan Event
	quit      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	connected atomic.Bool

	mu      sync.Mutex
	conn    net.Conn
	pending map[string]chan Event // ActionID → waiting caller
}

// NewClient creates a new Client. Call Start to begin connecting.
func NewClient(nodeID int64, host string, port int, user, pass string) *Client {
	return &Client{
		nodeID:  nodeID,
		host:    host,
		port:    port,
		user:    user,
		pass:    pass,
		events:  make(chan Event, 128),
		quit:    make(chan struct{}),
		pending: make(map[string]chan Event),
	}
}

// Start begins the reconnect loop in a background goroutine (idempotent).
func (c *Client) Start(ctx context.Context) {
	c.startOnce.Do(func() { go c.reconnectLoop(ctx) })
}

// Stop shuts down the client and closes the connection.
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		close(c.quit)
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
	})
}

// Events returns the read-only channel of AMI events.
func (c *Client) Events() <-chan Event { return c.events }

// IsConnected reports whether the AMI session is currently authenticated.
func (c *Client) IsConnected() bool { return c.connected.Load() }

// SendAction writes an AMI action to the current connection and returns
// immediately without waiting for a response.
func (c *Client) SendAction(headers map[string]string) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("AMI not connected (node %d)", c.nodeID)
	}
	return writeAction(conn, headers)
}

// SendActionWait sends an AMI action with a unique ActionID and waits up to
// timeout for the corresponding response block. Returns an error if the
// response indicates failure or the timeout elapses.
func (c *Client) SendActionWait(headers map[string]string, timeout time.Duration) (Event, error) {
	actionID := fmt.Sprintf("ya-%d", time.Now().UnixNano())
	headers["ActionID"] = actionID

	ch := make(chan Event, 1)
	c.mu.Lock()
	conn := c.conn
	if conn != nil {
		c.pending[actionID] = ch
	}
	c.mu.Unlock()

	if conn == nil {
		return Event{}, fmt.Errorf("AMI not connected (node %d)", c.nodeID)
	}
	defer func() {
		c.mu.Lock()
		delete(c.pending, actionID)
		c.mu.Unlock()
	}()

	if err := writeAction(conn, headers); err != nil {
		return Event{}, err
	}

	select {
	case evt := <-ch:
		if evt.Get("Response") == "Error" {
			return evt, fmt.Errorf("AMI error: %s", evt.Get("Message"))
		}
		return evt, nil
	case <-time.After(timeout):
		return Event{}, fmt.Errorf("timeout waiting for AMI response")
	}
}

// writeAction serialises an AMI action block to the connection.
// The Action header is always written first as required by the AMI protocol.
func writeAction(conn net.Conn, headers map[string]string) error {
	var sb strings.Builder
	// Action must be the first header per the AMI protocol spec.
	if v, ok := headers["Action"]; ok {
		fmt.Fprintf(&sb, "Action: %s\r\n", v)
	}
	for k, v := range headers {
		if k == "Action" {
			continue
		}
		fmt.Fprintf(&sb, "%s: %s\r\n", k, v)
	}
	sb.WriteString("\r\n")
	_, err := conn.Write([]byte(sb.String()))
	return err
}

func (c *Client) reconnectLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-c.quit:
			return
		case <-ctx.Done():
			return
		default:
		}

		if err := c.connect(ctx); err != nil {
			select {
			case <-c.quit:
				return
			case <-ctx.Done():
				return
			default:
			}
			slog.Warn("AMI connect error", "node_id", c.nodeID,
				"host", c.host, "err", err, "retry_in", backoff)
			select {
			case <-c.quit:
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second // reset after clean disconnect
	}
}

func (c *Client) connect(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		c.connected.Store(false)
		conn.Close()
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	r := bufio.NewReader(conn)

	// Read the AMI banner (e.g. "Asterisk Call Manager/1.3").
	banner, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read banner: %w", err)
	}
	slog.Debug("AMI banner", "node_id", c.nodeID, "banner", strings.TrimSpace(banner))

	if err := writeAction(conn, map[string]string{
		"Action":   "Login",
		"ActionID": "auth-1",
		"Username": c.user,
		"Secret":   c.pass,
	}); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	return c.readLoop(ctx, r)
}

// readLoop reads blank-line-delimited AMI message blocks until the connection closes.
func (c *Client) readLoop(ctx context.Context, r *bufio.Reader) error {
	headers := make(map[string]string)

	for {
		select {
		case <-c.quit:
			return nil
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if len(headers) == 0 {
				continue
			}
			evt := Event{Headers: headers}
			headers = make(map[string]string)

			// Route responses with an ActionID to waiting callers.
			if id := evt.Get("ActionID"); id != "" {
				c.mu.Lock()
				ch, ok := c.pending[id]
				c.mu.Unlock()
				if ok {
					select {
					case ch <- evt:
					default:
					}
					continue
				}
			}

			// Handle login response inline.
			if evt.IsResponse() {
				switch evt.Get("Response") {
				case "Success":
					if evt.Get("Message") == "Authentication accepted" {
						c.connected.Store(true)
						slog.Info("AMI authenticated", "node_id", c.nodeID, "host", c.host)
					}
				case "Error":
					return fmt.Errorf("AMI auth failed: %s", evt.Get("Message"))
				}
			}

			select {
			case c.events <- evt:
			default:
				// Drop if consumer is not keeping up.
			}
			continue
		}

		if idx := strings.Index(line, ": "); idx > 0 {
			headers[line[:idx]] = line[idx+2:]
		}
	}
}

// TestConnection dials the AMI endpoint, sends a Login action, and returns nil
// on successful authentication or an error with a descriptive message.
// The connection is closed immediately after the result is known.
func TestConnection(host string, port int, user, pass string) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connection refused: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck

	r := bufio.NewReader(conn)
	if _, err := r.ReadString('\n'); err != nil {
		return fmt.Errorf("no AMI banner: %w", err)
	}

	if err := writeAction(conn, map[string]string{
		"Action":   "Login",
		"ActionID": "test-1",
		"Username": user,
		"Secret":   pass,
	}); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	headers := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(headers) == 0 {
				continue
			}
			if headers["Response"] == "Success" {
				return nil
			}
			msg := headers["Message"]
			if msg == "" {
				msg = "authentication failed"
			}
			return fmt.Errorf("%s", msg)
		}
		if idx := strings.Index(line, ": "); idx > 0 {
			headers[line[:idx]] = line[idx+2:]
		}
	}
}
