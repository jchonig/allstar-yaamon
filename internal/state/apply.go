// Package state implements declarative configuration via a YAML state file.
// It is used by the `yaamon apply` CLI command and the Docker entrypoint.
package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

// StateFile is the top-level structure of a state.yaml file.
type StateFile struct {
	Purge PurgePolicy  `yaml:"purge"`
	Users []UserSpec   `yaml:"users"`
	Nodes []NodeSpec   `yaml:"nodes"`
}

type PurgePolicy struct {
	Users     bool `yaml:"users"`
	Nodes     bool `yaml:"nodes"`
	Favorites bool `yaml:"favorites"`
}

type UserSpec struct {
	Username   string `yaml:"username"`
	Permission string `yaml:"permission"`
	Password   string `yaml:"password"`
}

type NodeSpec struct {
	Name       string         `yaml:"name"`
	NodeNumber string         `yaml:"node_number"`
	AMIHost    string         `yaml:"ami_host"`
	AMIPort    int            `yaml:"ami_port"`
	AMIUser    string         `yaml:"ami_user"`
	AMIPass    string         `yaml:"ami_pass"`
	Enabled    bool           `yaml:"enabled"`
	Favorites  []FavoriteSpec `yaml:"favorites"`
}

type FavoriteSpec struct {
	NodeNumber  string `yaml:"node_number"`
	Callsign    string `yaml:"callsign"`
	Description string `yaml:"description"`
	Location    string `yaml:"location"`
	GroupName   string `yaml:"group_name"`
}

// ApplyOptions controls apply behaviour.
type ApplyOptions struct {
	DryRun         bool
	ResetPasswords bool
	NoConfirm      bool
}

// ChangeReport summarises what was applied.
type ChangeReport struct {
	UsersCreated   int
	UsersUpdated   int
	UsersDeleted   int
	NodesCreated   int
	NodesUpdated   int
	NodesDeleted   int
	FavsCreated    int
	FavsUpdated    int
	FavsDeleted    int
}

func (r ChangeReport) Total() int {
	return r.UsersCreated + r.UsersUpdated + r.UsersDeleted +
		r.NodesCreated + r.NodesUpdated + r.NodesDeleted +
		r.FavsCreated + r.FavsUpdated + r.FavsDeleted
}

func (r ChangeReport) String() string {
	if r.Total() == 0 {
		return "No changes"
	}
	parts := []string{}
	if n := r.UsersCreated + r.UsersUpdated + r.UsersDeleted; n > 0 {
		parts = append(parts, fmt.Sprintf("users: +%d ~%d -%d", r.UsersCreated, r.UsersUpdated, r.UsersDeleted))
	}
	if n := r.NodesCreated + r.NodesUpdated + r.NodesDeleted; n > 0 {
		parts = append(parts, fmt.Sprintf("nodes: +%d ~%d -%d", r.NodesCreated, r.NodesUpdated, r.NodesDeleted))
	}
	if n := r.FavsCreated + r.FavsUpdated + r.FavsDeleted; n > 0 {
		parts = append(parts, fmt.Sprintf("favorites: +%d ~%d -%d", r.FavsCreated, r.FavsUpdated, r.FavsDeleted))
	}
	return fmt.Sprintf("Applied %d changes (%s)", r.Total(), strings.Join(parts, "; "))
}

// Load reads and parses a state file, substituting $ENV_VAR references.
func Load(path string) (*StateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var sf StateFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if err := substituteEnv(&sf); err != nil {
		return nil, err
	}
	return &sf, nil
}

// substituteEnv replaces $VAR_NAME values with their environment variable values.
func substituteEnv(sf *StateFile) error {
	for i, u := range sf.Users {
		v, err := resolveEnv(u.Password, fmt.Sprintf("users[%d].password", i))
		if err != nil {
			return err
		}
		sf.Users[i].Password = v
	}
	for i, n := range sf.Nodes {
		v, err := resolveEnv(n.AMIPass, fmt.Sprintf("nodes[%d].ami_pass", i))
		if err != nil {
			return err
		}
		sf.Nodes[i].AMIPass = v
	}
	return nil
}

func resolveEnv(value, field string) (string, error) {
	if !strings.HasPrefix(value, "$") {
		return value, nil
	}
	name := value[1:]
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("required env var %s is not set (referenced in %s)", name, field)
	}
	return val, nil
}

// Apply brings the database into the state described by sf.
func Apply(ctx context.Context, database *db.DB, sf *StateFile, opts ApplyOptions) (ChangeReport, error) {
	var report ChangeReport
	var err error

	report, err = applyUsers(ctx, database, sf, opts, report)
	if err != nil {
		return report, fmt.Errorf("applying users: %w", err)
	}

	report, err = applyNodes(ctx, database, sf, opts, report)
	if err != nil {
		return report, fmt.Errorf("applying nodes: %w", err)
	}

	return report, nil
}

func applyUsers(ctx context.Context, database *db.DB, sf *StateFile, opts ApplyOptions, r ChangeReport) (ChangeReport, error) {
	wantedByName := make(map[string]UserSpec, len(sf.Users))
	for _, u := range sf.Users {
		if u.Username == "" {
			return r, errors.New("user entry missing username")
		}
		if !db.ValidPermission(u.Permission) {
			return r, fmt.Errorf("user %q: invalid permission %q", u.Username, u.Permission)
		}
		wantedByName[u.Username] = u
	}

	for _, spec := range sf.Users {
		existing, err := database.GetUser(ctx, spec.Username)
		if errors.Is(err, db.ErrNotFound) {
			if opts.DryRun {
				fmt.Printf("  [dry-run] would create user %q (%s)\n", spec.Username, spec.Permission)
				r.UsersCreated++
				continue
			}
			hash, err := auth.HashPassword(spec.Password)
			if err != nil {
				return r, fmt.Errorf("hashing password for %q: %w", spec.Username, err)
			}
			if _, err := database.CreateUser(ctx, spec.Username, hash, spec.Permission); err != nil {
				return r, fmt.Errorf("creating user %q: %w", spec.Username, err)
			}
			fmt.Printf("  created user %q (%s)\n", spec.Username, spec.Permission)
			r.UsersCreated++
			continue
		}
		if err != nil {
			return r, err
		}

		changed := false
		if existing.Permission != spec.Permission {
			if !opts.DryRun {
				if err := database.UpdateUserPermission(ctx, existing.ID, spec.Permission); err != nil {
					return r, fmt.Errorf("updating permission for %q: %w", spec.Username, err)
				}
			}
			fmt.Printf("  updated user %q permission: %s → %s\n", spec.Username, existing.Permission, spec.Permission)
			changed = true
		}
		if opts.ResetPasswords {
			if !opts.DryRun {
				hash, err := auth.HashPassword(spec.Password)
				if err != nil {
					return r, fmt.Errorf("hashing password for %q: %w", spec.Username, err)
				}
				if err := database.UpdateUserPassword(ctx, existing.ID, hash); err != nil {
					return r, fmt.Errorf("updating password for %q: %w", spec.Username, err)
				}
			}
			fmt.Printf("  reset password for user %q\n", spec.Username)
			changed = true
		}
		if changed {
			r.UsersUpdated++
		}
	}

	if sf.Purge.Users {
		existing, err := database.ListUsers(ctx)
		if err != nil {
			return r, err
		}
		for _, u := range existing {
			if _, wanted := wantedByName[u.Username]; wanted {
				continue
			}
			// Never delete the last superuser.
			if u.Permission == db.PermSuperuser {
				n, _ := database.CountSuperusers(ctx)
				if n <= 1 {
					fmt.Printf("  skipping purge of %q: last superuser\n", u.Username)
					continue
				}
			}
			if opts.DryRun {
				fmt.Printf("  [dry-run] would delete user %q\n", u.Username)
			} else {
				if err := database.DeleteUser(ctx, u.ID); err != nil {
					return r, fmt.Errorf("deleting user %q: %w", u.Username, err)
				}
				fmt.Printf("  deleted user %q\n", u.Username)
			}
			r.UsersDeleted++
		}
	}

	return r, nil
}

func applyNodes(ctx context.Context, database *db.DB, sf *StateFile, opts ApplyOptions, r ChangeReport) (ChangeReport, error) {
	wantedByNumber := make(map[string]NodeSpec, len(sf.Nodes))
	for _, n := range sf.Nodes {
		if n.NodeNumber == "" {
			return r, errors.New("node entry missing node_number")
		}
		wantedByNumber[n.NodeNumber] = n
	}

	for _, spec := range sf.Nodes {
		if spec.AMIPort == 0 {
			spec.AMIPort = 5038
		}
		if spec.AMIHost == "" {
			spec.AMIHost = "localhost"
		}

		existing, err := database.GetNodeByNumber(ctx, spec.NodeNumber)
		if errors.Is(err, db.ErrNotFound) {
			if opts.DryRun {
				fmt.Printf("  [dry-run] would create node %q (%s)\n", spec.Name, spec.NodeNumber)
				r.NodesCreated++
			} else {
				n, err := database.CreateNode(ctx, db.Node{
					Name: spec.Name, NodeNumber: spec.NodeNumber,
					AMIHost: spec.AMIHost, AMIPort: spec.AMIPort,
					AMIUser: spec.AMIUser, AMIPass: spec.AMIPass,
					Enabled: spec.Enabled,
				})
				if err != nil {
					return r, fmt.Errorf("creating node %q: %w", spec.NodeNumber, err)
				}
				fmt.Printf("  created node %q (%s)\n", spec.Name, spec.NodeNumber)
				r.NodesCreated++
				r, err = applyFavorites(ctx, database, n.ID, spec, opts, sf.Purge.Favorites, r)
				if err != nil {
					return r, err
				}
			}
			continue
		}
		if err != nil {
			return r, err
		}

		update := db.Node{
			ID: existing.ID, Name: spec.Name, NodeNumber: spec.NodeNumber,
			AMIHost: spec.AMIHost, AMIPort: spec.AMIPort,
			AMIUser: spec.AMIUser, AMIPass: spec.AMIPass,
			Enabled: spec.Enabled,
		}
		if nodeChanged(*existing, update) {
			if opts.DryRun {
				fmt.Printf("  [dry-run] would update node %q (%s)\n", spec.Name, spec.NodeNumber)
			} else {
				if err := database.UpdateNode(ctx, update); err != nil {
					return r, fmt.Errorf("updating node %q: %w", spec.NodeNumber, err)
				}
				fmt.Printf("  updated node %q (%s)\n", spec.Name, spec.NodeNumber)
			}
			r.NodesUpdated++
		}

		r, err = applyFavorites(ctx, database, existing.ID, spec, opts, sf.Purge.Favorites, r)
		if err != nil {
			return r, err
		}
	}

	if sf.Purge.Nodes {
		existing, err := database.ListNodes(ctx)
		if err != nil {
			return r, err
		}
		for _, n := range existing {
			if _, wanted := wantedByNumber[n.NodeNumber]; wanted {
				continue
			}
			if opts.DryRun {
				fmt.Printf("  [dry-run] would delete node %q (%s)\n", n.Name, n.NodeNumber)
			} else {
				if err := database.DeleteNode(ctx, n.ID); err != nil {
					return r, fmt.Errorf("deleting node %q: %w", n.NodeNumber, err)
				}
				fmt.Printf("  deleted node %q (%s)\n", n.Name, n.NodeNumber)
			}
			r.NodesDeleted++
		}
	}

	return r, nil
}

func applyFavorites(ctx context.Context, database *db.DB, nodeID int64, spec NodeSpec, opts ApplyOptions, purgeFavs bool, r ChangeReport) (ChangeReport, error) {
	wantedByNumber := make(map[string]FavoriteSpec, len(spec.Favorites))
	for i, f := range spec.Favorites {
		if f.NodeNumber == "" {
			return r, fmt.Errorf("node %q favorites[%d]: missing node_number", spec.NodeNumber, i)
		}
		if f.GroupName == "" {
			spec.Favorites[i].GroupName = "default"
		}
		wantedByNumber[f.NodeNumber] = spec.Favorites[i]
	}

	for _, fspec := range spec.Favorites {
		if fspec.GroupName == "" {
			fspec.GroupName = "default"
		}
		existing, err := database.GetFavoriteByNodeNumber(ctx, nodeID, fspec.NodeNumber)
		if errors.Is(err, db.ErrNotFound) {
			if opts.DryRun {
				fmt.Printf("    [dry-run] would create favorite %s in node %s\n", fspec.NodeNumber, spec.NodeNumber)
				r.FavsCreated++
				continue
			}
			if _, err := database.CreateFavorite(ctx, db.Favorite{
				NodeID: nodeID, NodeNumber: fspec.NodeNumber,
				Callsign: fspec.Callsign, Description: fspec.Description,
				Location: fspec.Location, GroupName: fspec.GroupName,
			}); err != nil {
				return r, fmt.Errorf("creating favorite %s: %w", fspec.NodeNumber, err)
			}
			r.FavsCreated++
			continue
		}
		if err != nil {
			return r, err
		}
		update := db.Favorite{
			ID: existing.ID, NodeID: nodeID, NodeNumber: fspec.NodeNumber,
			Callsign: fspec.Callsign, Description: fspec.Description,
			Location: fspec.Location, GroupName: fspec.GroupName,
			SortOrder: existing.SortOrder,
		}
		if favChanged(*existing, update) {
			if opts.DryRun {
				fmt.Printf("    [dry-run] would update favorite %s in node %s\n", fspec.NodeNumber, spec.NodeNumber)
			} else {
				if err := database.UpdateFavorite(ctx, update); err != nil {
					return r, fmt.Errorf("updating favorite %s: %w", fspec.NodeNumber, err)
				}
			}
			r.FavsUpdated++
		}
	}

	if !purgeFavs {
		return r, nil
	}

	existing, err := database.ListFavoritesByNode(ctx, nodeID)
	if err != nil {
		return r, err
	}
	for _, f := range existing {
		if _, wanted := wantedByNumber[f.NodeNumber]; wanted {
			continue
		}
		if opts.DryRun {
			fmt.Printf("    [dry-run] would delete favorite %s from node %s\n", f.NodeNumber, spec.NodeNumber)
		} else {
			if err := database.DeleteFavorite(ctx, f.ID); err != nil {
				return r, fmt.Errorf("deleting favorite %s: %w", f.NodeNumber, err)
			}
		}
		r.FavsDeleted++
	}

	return r, nil
}

func nodeChanged(a, b db.Node) bool {
	return a.Name != b.Name || a.AMIHost != b.AMIHost || a.AMIPort != b.AMIPort ||
		a.AMIUser != b.AMIUser || a.AMIPass != b.AMIPass || a.Enabled != b.Enabled
}

func favChanged(a, b db.Favorite) bool {
	return a.Callsign != b.Callsign || a.Description != b.Description ||
		a.Location != b.Location || a.GroupName != b.GroupName
}
