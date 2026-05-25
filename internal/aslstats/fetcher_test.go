package aslstats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestFetcher(srv *httptest.Server) *Fetcher {
	return &Fetcher{
		baseURL: srv.URL,
		client:  srv.Client(),
		limiter: newRateLimiter(100),
	}
}

func TestFetchParsesAPIResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {
				"callsign": "W1AW",
				"access_webtransceiver": 1,
				"server": {"Affiliation": "ARRL HQ", "SiteName": "Fallback"}
			},
			"stats": {
				"data": {
					"keyed": true,
					"totaltxtime": 123.45,
					"totalkeyups": 7,
					"links": ["12345", "67890"]
				}
			}
		}`))
	}))
	defer srv.Close()

	f := newTestFetcher(srv)
	s := f.Fetch(context.Background(), "99999")

	if s.Error != "" {
		t.Fatalf("unexpected error: %s", s.Error)
	}
	if s.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", s.Callsign)
	}
	if s.Description != "ARRL HQ" {
		t.Errorf("Description = %q, want ARRL HQ (from Affiliation)", s.Description)
	}
	if !s.Keyed {
		t.Error("Keyed = false, want true")
	}
	if s.TotalTxTime != 123.45 {
		t.Errorf("TotalTxTime = %v, want 123.45", s.TotalTxTime)
	}
	if s.TotalKeyups != 7 {
		t.Errorf("TotalKeyups = %d, want 7", s.TotalKeyups)
	}
	if s.ConnectedLinks != 2 {
		t.Errorf("ConnectedLinks = %d, want 2", s.ConnectedLinks)
	}
	if len(s.LinkedNodes) != 2 || s.LinkedNodes[0] != "12345" || s.LinkedNodes[1] != "67890" {
		t.Errorf("LinkedNodes = %v, want [12345 67890]", s.LinkedNodes)
	}
	if !s.Web {
		t.Error("Web = false, want true (access_webtransceiver=1)")
	}
}

func TestFetchDescriptionFallsBackToSiteName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {
				"callsign": "K0EX",
				"server": {"Affiliation": "", "SiteName": "Example Site"}
			},
			"stats": {"data": {}}
		}`))
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "11111")
	if s.Error != "" {
		t.Fatalf("unexpected error: %s", s.Error)
	}
	if s.Description != "Example Site" {
		t.Errorf("Description = %q, want Example Site (SiteName fallback)", s.Description)
	}
}

func TestFetchHandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "00000")
	if s.Error == "" {
		t.Error("expected error for 404 response, got none")
	}
}

func TestFetchHandlesEmptyLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {"callsign": "N0CALL", "server": {}},
			"stats": {"data": {"keyed": false, "links": []}}
		}`))
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "55555")
	if s.Error != "" {
		t.Fatalf("unexpected error: %s", s.Error)
	}
	if s.ConnectedLinks != 0 {
		t.Errorf("ConnectedLinks = %d, want 0", s.ConnectedLinks)
	}
	if len(s.LinkedNodes) != 0 {
		t.Errorf("LinkedNodes = %v, want empty", s.LinkedNodes)
	}
}

func TestFetchNodeNumberPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"node": {}, "stats": {"data": {}}}`))
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "42042")
	if s.NodeNumber != "42042" {
		t.Errorf("NodeNumber = %q, want 42042", s.NodeNumber)
	}
}
