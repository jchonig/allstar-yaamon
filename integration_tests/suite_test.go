//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// baseURL is the URL of the running yaamon container, set by make test-integration.
var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("YAAMON_TEST_URL")
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "YAAMON_TEST_URL not set — run via 'make test-integration'")
		os.Exit(1)
	}
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "server not ready: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server at %s not ready after %s", url, timeout)
}

func get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}
