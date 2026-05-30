package qrz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const callookValid = `{
  "status": "VALID",
  "type": "PERSON",
  "current": {"callsign": "W1AW", "operClass": "A"},
  "name": "ARRL HDQTRS",
  "address": {
    "line1": "225 MAIN ST",
    "line2": "NEWINGTON, CT  06111",
    "country": "United States"
  }
}`

const callookInvalid = `{"status": "INVALID"}`

// interceptCallook redirects http.DefaultClient requests to the given test server URL.
func interceptCallook(t *testing.T, target string) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = &singleHostTransport{target: target, orig: orig}
	t.Cleanup(func() { http.DefaultTransport = orig })
}

func TestLookupCallook_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(callookValid))
	}))
	defer srv.Close()
	interceptCallook(t, srv.URL)

	c := New("", "")
	rec, err := c.LookupCallook(context.Background(), "W1AW", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", rec.Callsign)
	}
	if rec.State != "CT" {
		t.Errorf("State = %q, want CT", rec.State)
	}
	if rec.Class != "A" {
		t.Errorf("Class = %q, want A", rec.Class)
	}
}

func TestLookupCallook_Invalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(callookInvalid))
	}))
	defer srv.Close()
	interceptCallook(t, srv.URL)

	c := New("", "")
	_, err := c.LookupCallook(context.Background(), "KK9XXX", nil)
	if err == nil {
		t.Fatal("expected error for INVALID status, got nil")
	}
}

func TestLookupCallook_CacheHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(callookValid))
	}))
	defer srv.Close()
	interceptCallook(t, srv.URL)

	c := New("", "")
	if _, err := c.LookupCallook(context.Background(), "W1AW", nil); err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	if _, err := c.LookupCallook(context.Background(), "W1AW", nil); err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if calls != 1 {
		t.Errorf("HTTP calls = %d, want 1 (second should be cached)", calls)
	}
}

func TestLookupCallook_PersistsRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(callookValid))
	}))
	defer srv.Close()
	interceptCallook(t, srv.URL)

	saved := map[string]json.RawMessage{}
	c := New("", "")
	if _, err := c.LookupCallook(context.Background(), "W1AW", &mockSaver{saved: saved}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := saved["W1AW"]; !ok {
		t.Error("expected record to be persisted via Saver")
	}
}

func TestSetCredentials(t *testing.T) {
	c := New("olduser", "oldpass")
	c.sessionKey = "some-session"
	c.SetCredentials("newuser", "newpass")
	if c.username != "newuser" || c.password != "newpass" {
		t.Error("credentials not updated")
	}
	if c.sessionKey != "" {
		t.Error("session key should be cleared on credential change")
	}
}

func TestParseState(t *testing.T) {
	cases := []struct{ in, want string }{
		{"NEWINGTON, CT  06111", "CT"},
		{"CHICAGO, IL  60601", "IL"},
		{"", ""},
		{"no comma here", ""},
	}
	for _, tc := range cases {
		got := parseState(tc.in)
		if got != tc.want {
			t.Errorf("parseState(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// singleHostTransport redirects all requests to a fixed test server URL.
type singleHostTransport struct {
	target string
	orig   http.RoundTripper
}

func (t *singleHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = t.target[len("http://"):]
	return t.orig.RoundTrip(req2)
}
