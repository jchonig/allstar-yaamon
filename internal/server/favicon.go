package server

import (
	"encoding/base64"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
)

const (
	configKeyFavicon = "custom_favicon"
	faviconMaxBytes  = 256 * 1024 // 256 KB
)

// serveFavicon serves the custom favicon from the DB, or falls back to the
// embedded static asset named by fallback.
func (s *Server) serveFavicon(fallback, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := s.db.GetConfig(r.Context(), configKeyFavicon)
		if err == nil && raw != "" {
			data, decErr := base64.StdEncoding.DecodeString(raw)
			if decErr == nil {
				w.Header().Set("Content-Type", "image/png")
				w.Header().Set("Cache-Control", "public, max-age=3600")
				w.Write(data) //nolint:errcheck
				return
			}
		}
		// Fall back to the embedded static file.
		staticFS, fsErr := fs.Sub(s.webFS, "web/static")
		if fsErr != nil {
			http.Error(w, "favicon not found", http.StatusNotFound)
			return
		}
		f, err2 := staticFS.Open(fallback)
		if err2 != nil {
			http.Error(w, "favicon not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		io.Copy(w, f) //nolint:errcheck
	}
}

// handleAPIUploadFavicon stores the uploaded image as a custom favicon.
// Body: raw image bytes (PNG or ICO), Content-Type must start with "image/".
func (s *Server) handleAPIUploadFavicon(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, faviconMaxBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) < 4 {
		http.Error(w, "file too small", http.StatusBadRequest)
		return
	}
	// Validate: PNG magic bytes (89 50 4E 47) or ICO magic (00 00 01 00) or JPEG (FF D8 FF).
	if !isPNG(data) && !isICO(data) && !isJPEG(data) {
		http.Error(w, "unsupported image format (PNG, ICO, or JPEG required)", http.StatusBadRequest)
		return
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	if err := s.db.SetConfig(r.Context(), configKeyFavicon, encoded); err != nil {
		http.Error(w, "store failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("custom favicon uploaded", "bytes", len(data))
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIDeleteFavicon removes the custom favicon, reverting to the embedded default.
func (s *Server) handleAPIDeleteFavicon(w http.ResponseWriter, r *http.Request) {
	if err := s.db.SetConfig(r.Context(), configKeyFavicon, ""); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("custom favicon removed")
	writeJSON(w, map[string]any{"ok": true})
}

func isPNG(b []byte)  bool { return len(b) >= 4 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 }
func isICO(b []byte)  bool { return len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0x01 && b[3] == 0x00 }
func isJPEG(b []byte) bool { return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF }
