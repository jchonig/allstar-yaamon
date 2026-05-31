package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
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
	ID             int64
	Username       string
	Password       string // bcrypt hash; "*" means local login disabled
	Permission     string
	FullName       string
	AvatarURL      string
	QRZUsername    string
	QRZPasswordEnc string
	LookupSource   string
	WebAuthnID     []byte
}

const userSelectCols = `id, username, password, permission, full_name, avatar_url, qrz_username, qrz_password_enc, lookup_source, webauthn_id`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Permission, &u.FullName, &u.AvatarURL,
		&u.QRZUsername, &u.QRZPasswordEnc, &u.LookupSource, &u.WebAuthnID)
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

// UpdateUserQRZ stores encrypted QRZ credentials for a user.
func (db *DB) UpdateUserQRZ(ctx context.Context, id int64, username, encPassword string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET qrz_username = ?, qrz_password_enc = ? WHERE id = ?`,
		username, encPassword, id)
	return err
}

// UpdateUserLookupSource saves the callsign lookup source preference for a user.
func (db *DB) UpdateUserLookupSource(ctx context.Context, id int64, source string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE users SET lookup_source = ? WHERE id = ?`, source, id)
	return err
}

// GetOrSetWebAuthnID returns the user's stable WebAuthn user handle, generating
// and persisting a 64-byte random one if it doesn't exist yet.
func (db *DB) GetOrSetWebAuthnID(ctx context.Context, userID int64) ([]byte, error) {
	var id []byte
	err := db.sql.QueryRowContext(ctx, `SELECT webauthn_id FROM users WHERE id = ?`, userID).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("get webauthn_id: %w", err)
	}
	if len(id) > 0 {
		return id, nil
	}
	id = make([]byte, 64)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("generate webauthn_id: %w", err)
	}
	if _, err := db.sql.ExecContext(ctx, `UPDATE users SET webauthn_id = ? WHERE id = ?`, id, userID); err != nil {
		return nil, fmt.Errorf("save webauthn_id: %w", err)
	}
	return id, nil
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
