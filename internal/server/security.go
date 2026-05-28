package server

import (
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

// nodeNumberRE matches valid AllStar node numbers: 4–10 digits, nothing else.
var nodeNumberRE = regexp.MustCompile(`^\d{4,10}$`)

// validNodeNumber reports whether s is a valid AllStar node number.
func validNodeNumber(s string) bool { return nodeNumberRE.MatchString(s) }

// --- HSTS middleware ---

// hstsMiddleware sets Strict-Transport-Security on every response.
// Only attach this to the HTTPS handler, never the HTTP redirect handler.
func hstsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// --- CSRF origin check ---

// csrfMiddleware rejects state-changing requests where the Origin header is
// present and its host does not match the request's Host (or X-Forwarded-Host).
// Requests without an Origin header (curl, API clients) are always allowed
// through — SameSite=Strict cookies already prevent browser-based CSRF.
func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := url.Parse(origin)
		if err != nil {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
			return
		}
		expected := r.Host
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			expected = fwd
		}
		if u.Host != expected {
			http.Error(w, "CSRF: origin mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Login rate limiter ---

const (
	loginMaxFailures = 5
	loginWindow      = time.Minute
)

type ipRecord struct {
	failures int
	resetAt  time.Time
}

// loginLimiter is a simple per-IP sliding-window failure counter.
// After loginMaxFailures failed attempts within loginWindow the IP is locked
// out until the window expires.
type loginLimiter struct {
	mu      sync.Mutex
	records map[string]*ipRecord
}

func newLoginLimiter() *loginLimiter {
	l := &loginLimiter{records: make(map[string]*ipRecord)}
	go l.sweep()
	return l
}

// Allow returns true if the IP is permitted to attempt a login right now.
func (l *loginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.records[ip]
	if rec == nil || time.Now().After(rec.resetAt) {
		return true
	}
	return rec.failures < loginMaxFailures
}

// RecordFailure increments the failure count for ip, starting or extending the window.
func (l *loginLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec := l.records[ip]
	if rec == nil || time.Now().After(rec.resetAt) {
		l.records[ip] = &ipRecord{failures: 1, resetAt: time.Now().Add(loginWindow)}
		return
	}
	rec.failures++
}

// RecordSuccess resets the failure count for ip on a successful login.
func (l *loginLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, ip)
}

// sweep periodically removes expired records to keep memory bounded.
func (l *loginLimiter) sweep() {
	t := time.NewTicker(5 * time.Minute)
	for range t.C {
		l.mu.Lock()
		now := time.Now()
		for ip, rec := range l.records {
			if now.After(rec.resetAt) {
				delete(l.records, ip)
			}
		}
		l.mu.Unlock()
	}
}
