package qrz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const successXML = `<?xml version="1.0" ?>
<QRZDatabase version="1.34">
  <Session>
    <Key>abc123</Key>
    <Count>1</Count>
  </Session>
  <Callsign>
    <call>W1AW</call>
    <fname>Hiram Percy</fname>
    <name>Maxim</name>
    <addr2>Newington, CT</addr2>
    <state>CT</state>
    <country>USA</country>
    <email>info@arrl.org</email>
    <class>A</class>
  </Callsign>
</QRZDatabase>`

const authXML = `<?xml version="1.0" ?>
<QRZDatabase version="1.34">
  <Session>
    <Key>abc123</Key>
  </Session>
</QRZDatabase>`

const errorXML = `<?xml version="1.0" ?>
<QRZDatabase version="1.34">
  <Session>
    <Error>Not found: KK9XXX</Error>
  </Session>
</QRZDatabase>`

func newTestClient(srv *httptest.Server) *Client {
	c := New("user", "pass")
	c.baseURL = srv.URL + "/"
	c.http = srv.Client()
	return c
}

func TestLookup_Success(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("username") != "" {
			// auth request
			w.Write([]byte(authXML))
			return
		}
		w.Write([]byte(successXML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rec, err := c.Lookup(context.Background(), "W1AW", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", rec.Callsign)
	}
	if rec.FirstName != "Hiram Percy" {
		t.Errorf("FirstName = %q, want Hiram Percy", rec.FirstName)
	}
	if rec.State != "CT" {
		t.Errorf("State = %q, want CT", rec.State)
	}
	if calls != 2 {
		t.Errorf("HTTP calls = %d, want 2 (auth + lookup)", calls)
	}
}

func TestLookup_CacheHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(authXML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.cache["W1AW"] = Record{Callsign: "W1AW", FetchedAt: time.Now()}

	rec, err := c.Lookup(context.Background(), "W1AW", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", rec.Callsign)
	}
	if calls != 0 {
		t.Errorf("HTTP calls = %d, want 0 (should be served from cache)", calls)
	}
}

func TestLookup_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("username") != "" {
			w.Write([]byte(authXML))
			return
		}
		w.Write([]byte(errorXML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Lookup(context.Background(), "KK9XXX", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSeed_SkipsStale(t *testing.T) {
	c := New("user", "pass")
	fresh := Record{Callsign: "W1AW", FetchedAt: time.Now()}
	stale := Record{Callsign: "K1ZZ", FetchedAt: time.Now().Add(-60 * 24 * time.Hour)}
	c.Seed(map[string]Record{
		"W1AW": fresh,
		"K1ZZ": stale,
	})
	if _, ok := c.cache["W1AW"]; !ok {
		t.Error("fresh record should be in cache")
	}
	if _, ok := c.cache["K1ZZ"]; ok {
		t.Error("stale record should not be seeded into cache")
	}
}

func TestLookup_PersistsRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("username") != "" {
			w.Write([]byte(authXML))
			return
		}
		w.Write([]byte(successXML))
	}))
	defer srv.Close()

	saved := map[string]json.RawMessage{}
	saver := &mockSaver{saved: saved}

	c := newTestClient(srv)
	_, err := c.Lookup(context.Background(), "W1AW", saver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := saved["W1AW"]; !ok {
		t.Error("expected record to be persisted via Saver")
	}
}

type mockSaver struct {
	saved map[string]json.RawMessage
}

func (m *mockSaver) SaveQRZRecord(_ context.Context, callsign string, record json.RawMessage, _ time.Time) error {
	m.saved[callsign] = record
	return nil
}
