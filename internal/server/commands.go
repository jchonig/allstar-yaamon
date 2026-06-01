package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

// computeCheck returns the first 16 hex characters of SHA-256("{index}:{cmdTemplate}").
// The client echoes this back with every POST so the server can detect stale clients
// after a config reload without embedding the raw command template in the page.
func computeCheck(i int, cmdTemplate string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", i, cmdTemplate)))
	return fmt.Sprintf("%x", h)[:16]
}

type commandArgJSON struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type commandListItem struct {
	Index int              `json:"index"`
	Name  string           `json:"name"`
	Check string           `json:"check"`
	Args  []commandArgJSON `json:"args"`
	Group string           `json:"group"`
}

// handleAPIListCommands returns the commands available to the calling user for a node.
// GET /api/nodes/{id}/commands
func (s *Server) handleAPIListCommands(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	if _, err := s.db.GetNodeByID(r.Context(), nodeID); err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	sess := auth.FromContext(r.Context())
	userPerm := ""
	if sess != nil {
		userPerm = sess.Permission
	}

	var items []commandListItem
	for i, cmd := range s.cfg.Commands.Commands {
		if !db.PermissionAtLeast(userPerm, cmd.Role) {
			continue
		}
		args := make([]commandArgJSON, 0, len(cmd.Args))
		for _, a := range cmd.Args {
			args = append(args, commandArgJSON{Name: a.Name, Label: a.Label, Type: a.Type})
		}
		items = append(items, commandListItem{
			Index: i,
			Name:  cmd.Name,
			Check: computeCheck(i, cmd.Cmd),
			Args:  args,
			Group: cmd.Group,
		})
	}
	if items == nil {
		items = []commandListItem{}
	}
	writeJSON(w, items)
}

type runCommandRequest struct {
	Index int               `json:"index"`
	Check string            `json:"check"`
	Args  map[string]string `json:"args"`
}

// handleAPIRunCommand executes a node command from the Functions menu.
// POST /api/nodes/{id}/cmd
func (s *Server) handleAPIRunCommand(w http.ResponseWriter, r *http.Request) {
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

	var body runCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	cmds := s.cfg.Commands.Commands
	if body.Index < 0 || body.Index >= len(cmds) {
		http.Error(w, "invalid command index", http.StatusBadRequest)
		return
	}
	cmd := cmds[body.Index]

	if computeCheck(body.Index, cmd.Cmd) != body.Check {
		http.Error(w, "check mismatch — reload the page", http.StatusBadRequest)
		return
	}

	sess := auth.FromContext(r.Context())
	userPerm := ""
	if sess != nil {
		userPerm = sess.Permission
	}
	if !db.PermissionAtLeast(userPerm, cmd.Role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Substitute {node} with the node's actual node number.
	resolved := strings.ReplaceAll(cmd.Cmd, "{node}", n.NodeNumber)

	// Substitute user-supplied args.
	if body.Args == nil {
		body.Args = map[string]string{}
	}
	for k, v := range body.Args {
		resolved = strings.ReplaceAll(resolved, "{"+k+"}", v)
	}

	// Reject if any placeholder remains after substitution.
	if strings.Contains(resolved, "{") {
		http.Error(w, "unresolved command placeholders", http.StatusBadRequest)
		return
	}

	slog.Info("AMI command", "node_id", nodeID, "cmd", resolved)
	resp, err := s.amiMgr.SendActionWait(nodeID, map[string]string{
		"Action":  "Command",
		"Command": resolved,
	}, 5*time.Second)
	if err != nil {
		slog.Warn("AMI command failed", "node_id", nodeID, "cmd", resolved, "err", err)
		// Asterisk returns this when a CLI command runs but produces no console output.
		// The command still executed — treat as success with empty output.
		if strings.Contains(err.Error(), "Command output follows but no following output") {
			writeJSON(w, map[string]any{"ok": true, "output": ""})
			return
		}
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	slog.Info("AMI command response", "node_id", nodeID, "cmd", resolved, "response", resp.Headers["Response"], "output", resp.Headers["Output"])
	writeJSON(w, map[string]any{"ok": true, "output": resp.Headers["Output"]})
}
