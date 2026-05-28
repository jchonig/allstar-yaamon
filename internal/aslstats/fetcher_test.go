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
		// Matches the real stats.allstarlink.org/api/stats/{node} shape.
		w.Write([]byte(`{
			"node": {
				"callsign": "W1AW",
				"access_webtransceiver": "1",
				"node_frequency": "ARRL HQ"
			},
			"stats": {
				"data": {
					"keyed": true,
					"totaltxtime": "123",
					"totalkeyups": "7",
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
		t.Errorf("Description = %q, want ARRL HQ (from node_frequency)", s.Description)
	}
	if !s.Web {
		t.Error("Web = false, want true (access_webtransceiver=\"1\")")
	}
	if !s.Keyed {
		t.Error("Keyed = false, want true")
	}
	if s.TotalTxTime != 123 {
		t.Errorf("TotalTxTime = %v, want 123 (parsed from string)", s.TotalTxTime)
	}
	if s.TotalKeyups != 7 {
		t.Errorf("TotalKeyups = %d, want 7 (parsed from string)", s.TotalKeyups)
	}
	if s.ConnectedLinks != 2 {
		t.Errorf("ConnectedLinks = %d, want 2", s.ConnectedLinks)
	}
	if len(s.LinkedNodes) != 2 || s.LinkedNodes[0] != "12345" || s.LinkedNodes[1] != "67890" {
		t.Errorf("LinkedNodes = %v, want [12345 67890]", s.LinkedNodes)
	}
}

func TestFetchWebFalseWhenZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {"callsign": "K0EX", "access_webtransceiver": "0", "node_frequency": ""},
			"stats": {"data": {}}
		}`))
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "11111")
	if s.Error != "" {
		t.Fatalf("unexpected error: %s", s.Error)
	}
	if s.Web {
		t.Error("Web = true, want false (access_webtransceiver=\"0\")")
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
			"node": {"callsign": "N0CALL", "access_webtransceiver": "0", "node_frequency": ""},
			"stats": {"data": {"keyed": false, "totaltxtime": "0", "totalkeyups": "0", "links": []}}
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

func TestFetchStringEncodedNumbers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"node": {"callsign": "W6XYZ", "access_webtransceiver": "0", "node_frequency": ""},
			"stats": {"data": {"totaltxtime": "9999", "totalkeyups": "42", "links": []}}
		}`))
	}))
	defer srv.Close()

	s := newTestFetcher(srv).Fetch(context.Background(), "77777")
	if s.Error != "" {
		t.Fatalf("unexpected error: %s", s.Error)
	}
	if s.TotalTxTime != 9999 {
		t.Errorf("TotalTxTime = %v, want 9999 (API returns string)", s.TotalTxTime)
	}
	if s.TotalKeyups != 42 {
		t.Errorf("TotalKeyups = %d, want 42 (API returns string)", s.TotalKeyups)
	}
}

func TestFetchBulkParsesAllNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stats/" {
			t.Errorf("unexpected path %q, want /api/stats/", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"11111": {
				"node": {"callsign": "W1AW", "access_webtransceiver": "1", "node_frequency": "ARRL HQ"},
				"stats": {"data": {"keyed": true, "totaltxtime": "10", "totalkeyups": "3", "links": ["22222"]}}
			},
			"22222": {
				"node": {"callsign": "N0CALL", "access_webtransceiver": "0", "node_frequency": "Test"},
				"stats": {"data": {"keyed": false, "totaltxtime": "0", "totalkeyups": "0", "links": 0}}
			}
		}`))
	}))
	defer srv.Close()

	results, err := newTestFetcher(srv).FetchBulk(context.Background())
	if err != nil {
		t.Fatalf("FetchBulk error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	n1, ok := results["11111"]
	if !ok {
		t.Fatal("missing node 11111")
	}
	if n1.Callsign != "W1AW" {
		t.Errorf("11111 Callsign = %q, want W1AW", n1.Callsign)
	}
	if !n1.Keyed {
		t.Error("11111 Keyed = false, want true")
	}
	if n1.ConnectedLinks != 1 || len(n1.LinkedNodes) != 1 || n1.LinkedNodes[0] != "22222" {
		t.Errorf("11111 links = %v, want [22222]", n1.LinkedNodes)
	}

	n2, ok := results["22222"]
	if !ok {
		t.Fatal("missing node 22222")
	}
	if n2.Callsign != "N0CALL" {
		t.Errorf("22222 Callsign = %q, want N0CALL", n2.Callsign)
	}
	if n2.ConnectedLinks != 0 {
		t.Errorf("22222 ConnectedLinks = %d, want 0 (links is integer 0)", n2.ConnectedLinks)
	}
}

func TestFetchBulkHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := newTestFetcher(srv).FetchBulk(context.Background())
	if err == nil {
		t.Error("expected error for 429 response, got nil")
	}
}

func TestFetchBulkConsistentTimestamp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"11111": {"node": {}, "stats": {"data": {}}},
			"22222": {"node": {}, "stats": {"data": {}}}
		}`))
	}))
	defer srv.Close()

	results, err := newTestFetcher(srv).FetchBulk(context.Background())
	if err != nil {
		t.Fatalf("FetchBulk error: %v", err)
	}
	t1 := results["11111"].FetchedAt
	t2 := results["22222"].FetchedAt
	if !t1.Equal(t2) {
		t.Errorf("FetchedAt not consistent across bulk results: %v vs %v", t1, t2)
	}
}
