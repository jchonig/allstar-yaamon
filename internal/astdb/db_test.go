package astdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	input := `2000|WB6NIL|ASL Public Hub|Los Angeles, CA
2001|WB6NIL|ASL Public Hub|Los Angeles, CA
2002|WB6NIL|AllStarLink Parrot|AWS US-EAST-1
28500|W5ALC|146.52|Oklahoma City, OK
99999|N0CALL|Test Node|
`
	nodes, err := parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 5 {
		t.Errorf("got %d nodes, want 5", len(nodes))
	}

	n, ok := nodes["2000"]
	if !ok {
		t.Fatal("node 2000 not found")
	}
	if n.Callsign != "WB6NIL" {
		t.Errorf("callsign = %q, want WB6NIL", n.Callsign)
	}
	if n.Description != "ASL Public Hub" {
		t.Errorf("description = %q, want ASL Public Hub", n.Description)
	}
	if n.Location != "Los Angeles, CA" {
		t.Errorf("location = %q, want Los Angeles, CA", n.Location)
	}

	n2, ok := nodes["28500"]
	if !ok {
		t.Fatal("node 28500 not found")
	}
	if n2.Callsign != "W5ALC" || n2.Description != "146.52" || n2.Location != "Oklahoma City, OK" {
		t.Errorf("28500 = %+v, want W5ALC/146.52/Oklahoma City, OK", n2)
	}

	// Node with missing location field
	n3 := nodes["99999"]
	if n3.Callsign != "N0CALL" {
		t.Errorf("99999 callsign = %q, want N0CALL", n3.Callsign)
	}
	if n3.Location != "" {
		t.Errorf("99999 location = %q, want empty", n3.Location)
	}
}

func TestParseEmptyLines(t *testing.T) {
	input := "\n\n2000|WB6NIL|Hub|\n\n"
	nodes, err := parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("got %d nodes, want 1", len(nodes))
	}
}

func TestLookup(t *testing.T) {
	db := New("", false)
	db.mu.Lock()
	db.nodes["12345"] = Node{Callsign: "W1AW", Description: "ARRL HQ", Location: "Newington, CT"}
	db.mu.Unlock()

	n, ok := db.Lookup("12345")
	if !ok {
		t.Fatal("Lookup(12345) not found")
	}
	if n.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", n.Callsign)
	}

	_, ok = db.Lookup("99999")
	if ok {
		t.Error("Lookup(99999) should not be found")
	}
}

func TestLoadFile(t *testing.T) {
	// Write enough entries to pass the 25-entry minimum guard.
	var sb strings.Builder
	for i := 1000; i < 1030; i++ {
		fmt.Fprintf(&sb, "%d|KD9ABC|Node %d|Chicago, IL\n", i, i)
	}
	f, err := os.CreateTemp(t.TempDir(), "astdb*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()

	db := New(f.Name(), false)
	db.loadFile()

	if db.Len() != 30 {
		t.Errorf("Len() = %d, want 30", db.Len())
	}
	n, ok := db.Lookup("1000")
	if !ok {
		t.Fatal("node 1000 not found after loadFile")
	}
	if n.Callsign != "KD9ABC" {
		t.Errorf("callsign = %q, want KD9ABC", n.Callsign)
	}
	// lastModified should be seeded from the file mtime.
	db.mu.RLock()
	lastMod := db.lastModified
	db.mu.RUnlock()
	if lastMod == "" {
		t.Error("lastModified not set after loadFile")
	}
}

func TestLoadFileMissing(t *testing.T) {
	// A missing file should be a no-op, not an error.
	db := New(filepath.Join(t.TempDir(), "nonexistent.txt"), false)
	db.loadFile()
	if db.Len() != 0 {
		t.Errorf("expected 0 nodes for missing file, got %d", db.Len())
	}
}

func TestLoadFileTooSmall(t *testing.T) {
	// A file with fewer than 25 entries should be ignored.
	f, err := os.CreateTemp(t.TempDir(), "astdb*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("1000|KD9ABC|Hub|Chicago, IL\n") //nolint:errcheck
	f.Close()

	db := New(f.Name(), false)
	db.loadFile()
	if db.Len() != 0 {
		t.Errorf("expected 0 nodes for undersized file, got %d", db.Len())
	}
}

func TestRefreshDownloadsAndSavesFile(t *testing.T) {
	// Build a response body large enough to pass the 25-entry guard.
	var sb strings.Builder
	for i := 1000; i < 1030; i++ {
		sb.WriteString(fmt.Sprintf("%d|W%dXY|Node %d|City, ST\n", i, i, i))
	}
	body := sb.String()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body)) //nolint:errcheck
	}))
	defer srv.Close()

	outFile := filepath.Join(t.TempDir(), "astdb.txt")
	db := New(outFile, true)
	db.client = &http.Client{
		Transport: rewriteHost(srv.URL),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db.refresh(ctx)

	if db.Len() < 25 {
		t.Errorf("expected ≥25 nodes after refresh, got %d", db.Len())
	}

	// File should have been written.
	if _, err := os.Stat(outFile); err != nil {
		t.Errorf("output file not written: %v", err)
	}

	// lastModified should be set from the response header.
	db.mu.RLock()
	lastMod := db.lastModified
	db.mu.RUnlock()
	if lastMod != "Mon, 01 Jan 2024 00:00:00 GMT" {
		t.Errorf("lastModified = %q, want Mon, 01 Jan 2024 00:00:00 GMT", lastMod)
	}
}

func TestRefreshNotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	db := New("", true)
	db.mu.Lock()
	db.nodes["9999"] = Node{Callsign: "W1AW"}
	db.lastModified = "Mon, 01 Jan 2024 00:00:00 GMT"
	db.mu.Unlock()
	db.client = &http.Client{Transport: rewriteHost(srv.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db.refresh(ctx)

	// Existing data must be untouched after a 304.
	if db.Len() != 1 {
		t.Errorf("expected 1 node preserved after 304, got %d", db.Len())
	}
}

func TestRefreshIgnoresSmallDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1000|W1AW|Hub|City, ST\n")) //nolint:errcheck
	}))
	defer srv.Close()

	db := New("", true)
	db.mu.Lock()
	db.nodes["9999"] = Node{Callsign: "SEED"}
	db.mu.Unlock()
	db.client = &http.Client{Transport: rewriteHost(srv.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db.refresh(ctx)

	// The suspiciously small download should be discarded; seed node preserved.
	if db.Len() != 1 {
		t.Errorf("expected 1 node preserved after small download, got %d", db.Len())
	}
}

func TestSaveFileAtomic(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "astdb.txt")
	db := New(outFile, true)

	body := []byte("1000|W1AW|Hub|City, ST\n")
	db.saveFile(body)

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not readable: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("file contents = %q, want %q", got, body)
	}

	// No temp files should remain in the directory.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".astdb-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestNoUpdateSkipsNetwork(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Write a valid file so loadFile has something to read.
	var sb strings.Builder
	for i := 1000; i < 1030; i++ {
		fmt.Fprintf(&sb, "%d|KD9ABC|Node %d|Chicago, IL\n", i, i)
	}
	f, err := os.CreateTemp(t.TempDir(), "astdb*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(sb.String()) //nolint:errcheck
	f.Close()

	db := New(f.Name(), false)
	db.client = &http.Client{Transport: rewriteHost(srv.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	db.Start(ctx, 50*time.Millisecond)

	// Give the ticker a chance to fire if it incorrectly started.
	<-ctx.Done()

	if called {
		t.Error("update=false: network request was made but should not have been")
	}
	if db.Len() != 30 {
		t.Errorf("update=false: expected 30 nodes loaded from file, got %d", db.Len())
	}
}

// rewriteHost returns a RoundTripper that rewrites all requests to target.
type rewriteHostRT struct {
	target string
	base   http.RoundTripper
}

func rewriteHost(target string) http.RoundTripper {
	return &rewriteHostRT{target: target, base: http.DefaultTransport}
}

func (rt *rewriteHostRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	parsed, _ := url.Parse(rt.target)
	r2.URL.Scheme = parsed.Scheme
	r2.URL.Host = parsed.Host
	return rt.base.RoundTrip(r2)
}
