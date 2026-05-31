package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	cookieName = "yaamon_session"
	// Sessions are valid for 7 days.
	sessionTTL = 7 * 24 * time.Hour
)

type contextKey int

const sessionKey contextKey = iota

// Session holds the authenticated user's data embedded in the cookie.
type Session struct {
	UserID          int64  `json:"uid"`
	Username        string `json:"u"`
	Permission      string `json:"p"`
	FullName        string `json:"fn,omitempty"`
	AvatarURL       string `json:"av,omitempty"`
	Expires         int64  `json:"exp"` // Unix timestamp
	AuthMethod      string `json:"am,omitempty"`
	ExternalUsername string `json:"eu,omitempty"`
}

// Manager handles session signing and verification.
type Manager struct {
	secret []byte
	secure bool // set true when TLS is enabled
}

// NewManager creates a session manager with the given HMAC secret.
func NewManager(secret []byte, secure bool) *Manager {
	return &Manager{secret: secret, secure: secure}
}

// IsSecure reports whether the session manager is configured for HTTPS-only cookies.
func (m *Manager) IsSecure() bool { return m.secure }

// GenerateSecret creates a random 32-byte secret suitable for HMAC.
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SetSession writes a signed session cookie to the response.
func (m *Manager) SetSession(w http.ResponseWriter, userID int64, username, permission, fullName, avatarURL string) error {
	s := Session{
		UserID:     userID,
		Username:   username,
		Permission: permission,
		FullName:   fullName,
		AvatarURL:  avatarURL,
		Expires:    time.Now().Add(sessionTTL).Unix(),
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return err
	}

	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := m.sign(encoded)
	value := encoded + "." + sig

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

// ClearSession removes the session cookie.
func (m *Manager) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// GetSession parses and verifies the session cookie from the request.
// Returns nil if there is no valid session.
func (m *Manager) GetSession(r *http.Request) *Session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	return m.parseToken(c.Value)
}

func (m *Manager) parseToken(token string) *Session {
	idx := strings.LastIndex(token, ".")
	if idx < 0 {
		return nil
	}
	encoded, sig := token[:idx], token[idx+1:]

	if !hmac.Equal([]byte(m.sign(encoded)), []byte(sig)) {
		return nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}

	var s Session
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil
	}

	if time.Now().Unix() > s.Expires {
		return nil
	}
	return &s
}

func (m *Manager) sign(data string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// FromContext retrieves the session from the request context (set by middleware).
func FromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey).(*Session)
	return s
}

// WithSession stores a session in the context. Used by proxy auth middleware.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}

// Middleware injects the session (if valid) into the request context.
// It does not overwrite a session already set by proxy auth middleware.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) == nil {
			if s := m.GetSession(r); s != nil {
				r = r.WithContext(WithSession(r.Context(), s))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission returns middleware that enforces a minimum permission level.
// Unauthenticated requests are redirected to /login.
// Insufficiently privileged requests get 403.
func (m *Manager) RequirePermission(minPerm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := FromContext(r.Context())
			if s == nil {
				http.Redirect(w, r, "/login?next="+r.URL.RequestURI(), http.StatusSeeOther)
				return
			}
			if !permissionAtLeast(s.Permission, minPerm) {
				http.Error(w, fmt.Sprintf("forbidden: requires %s permission", minPerm), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func permissionAtLeast(have, need string) bool {
	rank := map[string]int{
		"superuser": 4,
		"admin":     3,
		"readwrite": 2,
		"readonly":  1,
		"none":      0,
	}
	return rank[have] >= rank[need]
}
