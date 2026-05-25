// Package astdb downloads and caches the AllStar node database from allmondb.allstarlink.org.
// Format: NODE|CALLSIGN|DESCRIPTION|LOCATION  (pipe-delimited, one node per line)
package astdb

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const dbURL = "http://allmondb.allstarlink.org/"

// Node holds the static record for one AllStar node.
type Node struct {
	Callsign    string
	Description string
	Location    string
}

// DB holds an in-memory copy of the AllStar node database with periodic refresh.
type DB struct {
	mu           sync.RWMutex
	nodes        map[string]Node
	lastModified string
	client       *http.Client
}

// New creates an empty DB. Call Start to begin downloading.
func New() *DB {
	return &DB{
		nodes:  make(map[string]Node),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Lookup returns the node record for the given node number string.
func (db *DB) Lookup(nodeNumber string) (Node, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	n, ok := db.nodes[nodeNumber]
	return n, ok
}

// Len returns the number of nodes currently in the database.
func (db *DB) Len() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.nodes)
}

// Start downloads the database immediately, then refreshes every interval using
// If-Modified-Since so unchanged responses cost only a HEAD round-trip.
func (db *DB) Start(ctx context.Context, interval time.Duration) {
	db.refresh(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				db.refresh(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (db *DB) refresh(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dbURL, nil)
	if err != nil {
		slog.Warn("astdb: build request", "err", err)
		return
	}

	db.mu.RLock()
	lastMod := db.lastModified
	db.mu.RUnlock()

	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}

	resp, err := db.client.Do(req)
	if err != nil {
		slog.Warn("astdb: fetch", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		slog.Debug("astdb: not modified")
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("astdb: unexpected status", "status", resp.StatusCode)
		return
	}

	newMod := resp.Header.Get("Last-Modified")
	nodes, err := parse(resp.Body)
	if err != nil {
		slog.Warn("astdb: parse", "err", err)
		return
	}
	if len(nodes) < 25 {
		slog.Warn("astdb: suspiciously small download, ignoring", "count", len(nodes))
		return
	}

	db.mu.Lock()
	db.nodes = nodes
	if newMod != "" {
		db.lastModified = newMod
	}
	db.mu.Unlock()

	slog.Info("astdb: updated", "nodes", len(nodes))
}

// parse reads pipe-delimited astdb lines: NODE|CALLSIGN|DESCRIPTION|LOCATION
func parse(r io.Reader) (map[string]Node, error) {
	nodes := make(map[string]Node, 120000)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 2 {
			continue
		}
		num := strings.TrimSpace(parts[0])
		if num == "" {
			continue
		}
		n := Node{}
		if len(parts) > 1 {
			n.Callsign = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			n.Description = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			n.Location = strings.TrimSpace(parts[3])
		}
		nodes[num] = n
	}
	return nodes, sc.Err()
}
