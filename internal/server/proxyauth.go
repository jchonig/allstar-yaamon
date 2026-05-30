package server

import (
	"log/slog"
	"net/http"
	"strings"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
)

// proxyAuthMiddleware handles stateless per-request authentication from
// upstream proxy headers (Tailscale and OAuth2/oauth2-proxy).
// It runs before the cookie session middleware and, when it derives a session,
// stores it in the request context so the cookie middleware will not overwrite it.
func proxyAuthMiddleware(proxyCfg config.ProxyAuthConfig, tsCfg config.TailscaleAuthConfig, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tsCfg.Enabled {
				if sess, ok := tailscaleSession(r, tsCfg, database); ok {
					r = r.WithContext(auth.WithSession(r.Context(), sess))
					next.ServeHTTP(w, r)
					return
				}
			}

			if proxyCfg.Enabled {
				sess, deny := oauthSession(r, proxyCfg, database)
				if deny {
					http.Error(w, "forbidden: no matching group permission", http.StatusForbidden)
					return
				}
				if sess != nil {
					r = r.WithContext(auth.WithSession(r.Context(), sess))
					next.ServeHTTP(w, r)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// tailscaleSession attempts to derive a session from Tailscale identity headers.
// Returns (session, true) on success, (nil, false) when the header is absent or
// no matching DB user is found (fall through to login).
func tailscaleSession(r *http.Request, cfg config.TailscaleAuthConfig, database *db.DB) (*auth.Session, bool) {
	login := strings.TrimSpace(r.Header.Get(cfg.UserHeader))
	if login == "" {
		slog.Debug("tailscale auth: header absent", "header", cfg.UserHeader)
		return nil, false
	}

	u, err := database.GetUserByTailscaleLogin(r.Context(), login)
	if err != nil {
		slog.Debug("tailscale auth: no matching user", "login", login, "err", err)
		return nil, false
	}
	slog.Debug("tailscale auth: matched user", "login", login, "username", u.Username)

	sess := &auth.Session{
		UserID:     u.ID,
		Username:   u.Username,
		Permission: u.Permission,
		FullName:   u.FullName,
		AvatarURL:  u.AvatarURL,
		AuthMethod: "tailscale",
	}
	if login != u.Username {
		sess.ExternalUsername = login
	}

	// Opportunistically fill display name / avatar from headers when DB fields are empty.
	if sess.FullName == "" {
		if name := strings.TrimSpace(r.Header.Get(cfg.NameHeader)); name != "" {
			sess.FullName = name
		}
	}
	if sess.AvatarURL == "" {
		if pic := strings.TrimSpace(r.Header.Get(cfg.AvatarHeader)); pic != "" {
			sess.AvatarURL = pic
		}
	}

	return sess, true
}

// oauthSession attempts to derive a session from OAuth2/oauth2-proxy headers.
// Returns (session, false) on success, (nil, false) when the header is absent
// (fall through), and (nil, true) when a header is present but no group matches
// (deny with 403).
func oauthSession(r *http.Request, cfg config.ProxyAuthConfig, database *db.DB) (*auth.Session, bool) {
	username := strings.TrimSpace(r.Header.Get(cfg.UsernameHeader))
	if username == "" {
		slog.Debug("oauth2 auth: header absent", "header", cfg.UsernameHeader)
		return nil, false
	}

	groupsHdr := r.Header.Get(cfg.GroupsHeader)
	role, ok := highestRole(groupsHdr, cfg.GroupRoles)
	if !ok {
		if len(cfg.GroupRoles) == 0 {
			slog.Warn("oauth2 auth: group_roles is not configured — add a group_roles mapping to config.yaml to grant access", "username", username, "groups", groupsHdr)
		} else {
			slog.Warn("oauth2 auth: no matching group", "username", username, "groups", groupsHdr, "configured_groups", cfg.GroupRoles)
		}
		return nil, true // header present but no group matches → 403
	}
	slog.Debug("oauth2 auth: matched role", "username", username, "role", role)

	u, err := database.GetUser(r.Context(), username)
	if err != nil {
		if err != db.ErrNotFound {
			slog.Error("proxy auth: db lookup", "username", username, "err", err)
			return nil, true
		}
		if !cfg.CreateUsers {
			return nil, true
		}
		// Create a local-login-disabled account for this OAuth user.
		u, err = database.CreateUser(r.Context(), username, "*", role)
		if err != nil {
			slog.Error("proxy auth: create user", "username", username, "err", err)
			return nil, true
		}
		slog.Info("proxy auth: created user", "username", username, "role", role)
	}

	if cfg.UpdateDBRole && u.Permission != role {
		if dbErr := database.UpdateUserPermission(r.Context(), u.ID, role); dbErr != nil {
			slog.Warn("proxy auth: update db role", "username", username, "err", dbErr)
		} else {
			u.Permission = role
		}
	}

	return &auth.Session{
		UserID:     u.ID,
		Username:   u.Username,
		Permission: role,
		FullName:   u.FullName,
		AvatarURL:  u.AvatarURL,
		AuthMethod: "OAuth2",
	}, false
}

// highestRole returns the highest-ranked role found in the comma-separated
// groups header, using the configured group→role mapping.
func highestRole(groupsHdr string, groupRoles map[string]string) (string, bool) {
	rank := map[string]int{
		db.PermSuperuser: 4,
		db.PermAdmin:     3,
		db.PermReadWrite: 2,
		db.PermReadOnly:  1,
	}

	best := ""
	bestRank := -1
	for _, group := range strings.Split(groupsHdr, ",") {
		group = strings.TrimSpace(group)
		if role, ok := groupRoles[group]; ok {
			if r := rank[role]; r > bestRank {
				bestRank = r
				best = role
			}
		}
	}
	return best, bestRank >= 0
}
