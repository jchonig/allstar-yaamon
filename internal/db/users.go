package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Permission levels in descending order.
const (
	PermSuperuser = "superuser"
	PermAdmin     = "admin"
	PermReadWrite = "readwrite"
	PermReadOnly  = "readonly"
	PermNone      = "none"
)

var permRank = map[string]int{
	PermSuperuser: 4,
	PermAdmin:     3,
	PermReadWrite: 2,
	PermReadOnly:  1,
	PermNone:      0,
}

// PermissionAtLeast reports whether a has at least the access level of b.
func PermissionAtLeast(a, b string) bool {
	return permRank[a] >= permRank[b]
}

// ValidPermission reports whether p is a known permission level.
func ValidPermission(p string) bool {
	_, ok := permRank[p]
	return ok
}

type User struct {
	ID                 int64
	Username           string
	Password           string // bcrypt hash; "*" means local login disabled
	Permission         string
	FullName           string
	AvatarURL          string
	TailscaleUsernames string
}

const userSelectCols = `id, username, password, permission, full_name, avatar_url, tailscale_usernames`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Permission, &u.FullName, &u.AvatarURL, &u.TailscaleUsernames)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUser returns the user with the given username, or ErrNotFound.
func (db *DB) GetUser(ctx context.Context, username string) (*User, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users WHERE username = ?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// GetUserByID returns the user with the given id.
func (db *DB) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// GetUserByTailscaleLogin returns the user whose tailscale_usernames contains login, or ErrNotFound.
func (db *DB) GetUserByTailscaleLogin(ctx context.Context, login string) (*User, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users WHERE ','||tailscale_usernames||',' LIKE '%,'||?||',%'`, login)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by tailscale login: %w", err)
	}
	return u, nil
}

// ListUsers returns all users ordered by username.
func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT `+userSelectCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// CreateUser inserts a new user. password must already be bcrypt-hashed.
func (db *DB) CreateUser(ctx context.Context, username, passwordHash, permission string) (*User, error) {
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO users (username, password, permission) VALUES (?, ?, ?)`,
		username, passwordHash, permission,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, Password: passwordHash, Permission: permission}, nil
}

// UpdateUserProfile updates the full name and avatar URL for a user.
func (db *DB) UpdateUserProfile(ctx context.Context, id int64, fullName, avatarURL string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET full_name = ?, avatar_url = ? WHERE id = ?`, fullName, avatarURL, id)
	return err
}

// UpdateUserTailscaleUsernames updates the comma-separated Tailscale login list for a user.
// Returns an error if any login in the list is already assigned to a different user.
func (db *DB) UpdateUserTailscaleUsernames(ctx context.Context, id int64, usernames string) error {
	for _, login := range splitLogins(usernames) {
		var owner string
		err := db.sql.QueryRowContext(ctx,
			`SELECT username FROM users WHERE id != ? AND ','||tailscale_usernames||',' LIKE '%,'||?||',%'`,
			id, login,
		).Scan(&owner)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check tailscale conflict: %w", err)
		}
		if owner != "" {
			return fmt.Errorf("tailscale login %q is already assigned to user %q", login, owner)
		}
	}
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET tailscale_usernames = ? WHERE id = ?`, usernames, id)
	return err
}

func splitLogins(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if l := strings.TrimSpace(part); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// UpdateUserPassword sets a new bcrypt-hashed password.
func (db *DB) UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET password = ? WHERE id = ?`, passwordHash, id)
	return err
}

// UpdateUserPermission sets a new permission level.
func (db *DB) UpdateUserPermission(ctx context.Context, id int64, permission string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET permission = ? WHERE id = ?`, permission, id)
	return err
}

// DeleteUser deletes the user with the given id.
func (db *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// CountUsers returns the total number of users.
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CountSuperusers returns the number of superuser accounts.
func (db *DB) CountSuperusers(ctx context.Context) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE permission = 'superuser'`).Scan(&n)
	return n, err
}

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")
