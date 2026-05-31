// Package qrz fetches amateur radio callsign data from the QRZ.com XML API.
package qrz

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	DefaultBaseURL = "https://xmldata.qrz.com/xml/current/"
	CacheTTL       = 30 * 24 * time.Hour // 30 days
)

// Record holds the fields we extract from a callsign lookup.
type Record struct {
	Callsign  string    `json:"callsign" xml:"call"`
	FirstName string    `json:"first_name" xml:"fname"`
	LastName  string    `json:"last_name" xml:"name"`
	Address   string    `json:"address" xml:"addr2"`
	State     string    `json:"state" xml:"state"`
	Country   string    `json:"country" xml:"country"`
	Email     string    `json:"email" xml:"email"`
	Phone     string    `json:"phone" xml:"phone"`
	Class     string    `json:"class" xml:"class"`
	Source    string    `json:"source"`    // "qrz" or "callook"
	FetchedAt time.Time `json:"fetched_at"`
}

// Stale returns true if the record is older than CacheTTL.
func (r Record) Stale() bool {
	return time.Since(r.FetchedAt) > CacheTTL
}

type qrzSession struct {
	xml.Name `xml:"Session"`
	Key      string `xml:"Key"`
	Error    string `xml:"Error"`
}

type qrzCallsign struct {
	xml.Name `xml:"Callsign"`
	Record
}

type qrzResponse struct {
	xml.Name `xml:"QRZDatabase"`
	Session  qrzSession  `xml:"Session"`
	Callsign qrzCallsign `xml:"Callsign"`
}

// Saver is the subset of db.DB used to persist records.
type Saver interface {
	SaveQRZRecord(ctx context.Context, callsign string, record json.RawMessage, fetchedAt time.Time) error
}

// Client fetches callsign data from QRZ.com, with in-memory and SQLite caching.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client

	mu         sync.Mutex
	sessionKey string
	cache      map[string]Record
}

// New creates a Client. username and password are QRZ.com XML-access credentials.
func New(username, password string) *Client {
	return &Client{
		baseURL:  DefaultBaseURL,
		username: username,
		password: password,
		http:     &http.Client{Timeout: 10 * time.Second},
		cache:    make(map[string]Record),
	}
}

// ClearCache discards all in-memory cached records.
func (c *Client) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]Record)
}

// Seed pre-populates the in-memory cache from a DB snapshot (call on startup).
func (c *Client) Seed(records map[string]Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for call, rec := range records {
		if !rec.Stale() {
			c.cache[call] = rec
		}
	}
}

// Lookup returns QRZ data for callsign, using the cache when possible.
func (c *Client) Lookup(ctx context.Context, callsign string, saver Saver) (Record, error) {
	c.mu.Lock()
	if rec, ok := c.cache[callsign]; ok && !rec.Stale() {
		c.mu.Unlock()
		return rec, nil
	}
	c.mu.Unlock()

	rec, err := c.fetch(ctx, callsign)
	if err != nil {
		return Record{}, err
	}

	c.mu.Lock()
	c.cache[callsign] = rec
	c.mu.Unlock()

	if saver != nil {
		raw, merr := json.Marshal(rec)
		if merr == nil {
			if serr := saver.SaveQRZRecord(ctx, callsign, json.RawMessage(raw), rec.FetchedAt); serr != nil {
				slog.Warn("qrz: failed to persist record", "callsign", callsign, "err", serr)
			}
		}
	}

	return rec, nil
}

// Configured returns true if the client has credentials set.
func (c *Client) Configured() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username != "" && c.password != ""
}

func (c *Client) fetch(ctx context.Context, callsign string) (Record, error) {
	key, err := c.ensureSession(ctx)
	if err != nil {
		return Record{}, err
	}

	rec, err := c.lookupCallsign(ctx, key, callsign)
	if err != nil {
		// Session may have expired; retry once with a fresh session.
		c.mu.Lock()
		c.sessionKey = ""
		c.mu.Unlock()
		key, err2 := c.ensureSession(ctx)
		if err2 != nil {
			return Record{}, err
		}
		return c.lookupCallsign(ctx, key, callsign)
	}
	return rec, nil
}

func (c *Client) ensureSession(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.sessionKey != "" {
		k := c.sessionKey
		c.mu.Unlock()
		return k, nil
	}
	c.mu.Unlock()

	params := url.Values{
		"username": {c.username},
		"password": {c.password},
	}
	resp, err := c.doGet(ctx, params)
	if err != nil {
		return "", fmt.Errorf("qrz auth: %w", err)
	}
	if resp.Session.Error != "" {
		return "", fmt.Errorf("qrz auth: %s", resp.Session.Error)
	}
	if resp.Session.Key == "" {
		return "", fmt.Errorf("qrz auth: no session key returned")
	}

	c.mu.Lock()
	c.sessionKey = resp.Session.Key
	c.mu.Unlock()
	return resp.Session.Key, nil
}

func (c *Client) lookupCallsign(ctx context.Context, sessionKey, callsign string) (Record, error) {
	params := url.Values{
		"s": {sessionKey},
		"callsign": {callsign},
	}
	resp, err := c.doGet(ctx, params)
	if err != nil {
		return Record{}, fmt.Errorf("qrz lookup %s: %w", callsign, err)
	}
	if resp.Session.Error != "" {
		return Record{}, fmt.Errorf("qrz lookup %s: %s", callsign, resp.Session.Error)
	}
	rec := resp.Callsign.Record
	rec.FetchedAt = time.Now().UTC()
	rec.Source = "qrz"
	return rec, nil
}

func (c *Client) doGet(ctx context.Context, params url.Values) (*qrzResponse, error) {
	reqURL := c.baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var out qrzResponse
	if err := xml.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &out, nil
}
