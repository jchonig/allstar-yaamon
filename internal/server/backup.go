package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/backup"
	"allstar-yaamon/internal/db"
)

func (s *Server) handleBackupPage(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	nodes, _ := s.db.ListNodes(r.Context())
	data := struct {
		pageData
		Nodes interface{}
	}{pageData: s.newPageData()}
	fillSession(&data.pageData, sess)
	data.Nodes = nodes
	s.render(w, "backup", data)
}

// handleAPIBackup creates a backup and streams it as a file download.
// Body: {"passphrase": "secret"}  (passphrase is optional)
func (s *Server) handleAPIBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	data, manifest, err := backup.Create(r.Context(), s.db, Version, backup.CreateOptions{
		Passphrase: body.Passphrase,
	})
	if err != nil {
		slog.Error("backup create failed", "err", err)
		http.Error(w, "backup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("yaamon-%s.owbackup", manifest.CreatedAt.Format("20060102T150405Z"))
	slog.Info("backup created", "size", len(data), "encrypted", manifest.Encrypted, "filename", filename)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data) //nolint:errcheck
}

// handleAPIBackupInspect parses an uploaded .owbackup file and returns its manifest.
func (s *Server) handleAPIBackupInspect(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20) // 512 MB max
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}

	manifest, err := backup.Inspect(data)
	if err != nil {
		http.Error(w, "invalid backup file: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, manifest)
}

// handleAPIBackupRestore restores from an uploaded .owbackup file.
// Body is the raw .owbackup file bytes; passphrase (if needed) is in the
// X-Backup-Passphrase request header.
func (s *Server) handleAPIBackupRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}

	passphrase := r.Header.Get("X-Backup-Passphrase")

	preRestorePath, err := backup.Restore(r.Context(), s.db, Version, data, backup.RestoreOptions{
		Passphrase: passphrase,
	})
	if err != nil {
		slog.Error("restore failed", "err", err)
		http.Error(w, "restore failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("restore complete — restarting", "pre_restore_backup", preRestorePath)
	writeJSON(w, map[string]any{"ok": true, "pre_restore_backup": preRestorePath})

	// Exit after response is flushed so the container/systemd restarts the process.
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
}

// handleAPIFavoritesExport downloads favorites as a favorites.ini file.
func (s *Server) handleAPIFavoritesExport(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	n, err := s.db.GetNodeByID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	favs, err := s.db.ListFavoritesByNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	data := backup.ExportINI(favs, n.NodeNumber)
	filename := fmt.Sprintf("favorites-%s-%s.ini", n.NodeNumber, time.Now().UTC().Format("20060102"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write(data) //nolint:errcheck
}

// handleAPIFavoritesImportPreview parses an uploaded INI file and returns a dry-run summary.
func (s *Server) handleAPIFavoritesImportPreview(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	imported, err := backup.ParseINI(data)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Check which are new vs already exist.
	existing, _ := s.db.ListFavoritesByNode(r.Context(), nodeID)
	existSet := make(map[string]bool, len(existing))
	for _, f := range existing {
		existSet[f.NodeNumber] = true
	}

	var willAdd, willSkip int
	for _, f := range imported {
		if existSet[f.NodeNumber] {
			willSkip++
		} else {
			willAdd++
		}
	}
	writeJSON(w, map[string]any{
		"total":     len(imported),
		"will_add":  willAdd,
		"will_skip": willSkip,
	})
}

// handleAPIFavoritesImport commits an INI import (skipping existing entries).
func (s *Server) handleAPIFavoritesImport(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	imported, err := backup.ParseINI(data)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadRequest)
		return
	}

	existing, _ := s.db.ListFavoritesByNode(r.Context(), nodeID)
	existSet := make(map[string]bool, len(existing))
	for _, f := range existing {
		existSet[f.NodeNumber] = true
	}

	added := 0
	for _, f := range imported {
		if existSet[f.NodeNumber] {
			continue
		}
		if _, err := s.db.CreateFavorite(r.Context(), db.Favorite{
			NodeID:      nodeID,
			NodeNumber:  f.NodeNumber,
			Callsign:    f.Callsign,
			Description: f.Description,
		}); err != nil {
			slog.Warn("import favorite failed", "node_number", f.NodeNumber, "err", err)
			continue
		}
		added++
	}
	writeJSON(w, map[string]any{"added": added, "skipped": len(imported) - added})
}

// handleAPIImportAllmon3Preview parses an uploaded allmon3.ini and returns the nodes found.
// POST /api/admin/import/allmon3/preview — body: raw allmon3.ini bytes
func (s *Server) handleAPIImportAllmon3Preview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	nodes, err := backup.ParseAllmon3INI(data)
	if err != nil {
		http.Error(w, "parse error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(nodes) == 0 {
		http.Error(w, "no nodes found in file", http.StatusBadRequest)
		return
	}

	type nodePreview struct {
		backup.AllmonNode
		Exists bool `json:"exists"`
	}
	preview := make([]nodePreview, 0, len(nodes))
	for _, n := range nodes {
		_, err := s.db.GetNodeByNumber(r.Context(), n.NodeNumber)
		preview = append(preview, nodePreview{n, err == nil})
	}
	writeJSON(w, preview)
}

// handleAPIImportAllmon3 creates nodes from a selected subset of a parsed allmon3.ini.
// POST /api/admin/import/allmon3 — body: JSON array of AllmonNode (the selected ones)
func (s *Server) handleAPIImportAllmon3(w http.ResponseWriter, r *http.Request) {
	var nodes []backup.AllmonNode
	if err := json.NewDecoder(r.Body).Decode(&nodes); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	added := 0
	for _, n := range nodes {
		if n.AMIPort == 0 {
			n.AMIPort = 5038
		}
		if n.AMIHost == "" {
			n.AMIHost = "localhost"
		}
		created, err := s.db.CreateNode(r.Context(), db.Node{
			Name:       n.NodeNumber, // admin can rename afterwards
			NodeNumber: n.NodeNumber,
			AMIHost:    n.AMIHost,
			AMIPort:    n.AMIPort,
			AMIUser:    n.AMIUser,
			AMIPass:    n.AMIPass,
			Enabled:    true,
		})
		if err != nil {
			slog.Warn("allmon3 import: create node failed", "node", n.NodeNumber, "err", err)
			continue
		}
		s.amiMgr.Add(*created)
		added++
	}
	writeJSON(w, map[string]any{"added": added})
}
