package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Credential struct {
	ID             int64
	UserID         int64
	CredentialID   []byte
	Name           string
	CredentialJSON string
	CreatedAt      time.Time
	LastUsedAt     *time.Time
}

type WebAuthnSession struct {
	SessionID   string
	Ceremony    string
	UserID      *int64
	SessionJSON string
	ExpiresAt   time.Time
}

func (db *DB) CreateCredential(ctx context.Context, userID int64, credID []byte, name, credJSON string) (*Credential, error) {
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO webauthn_credentials (user_id, credential_id, name, credential_json) VALUES (?, ?, ?, ?)`,
		userID, credID, name, credJSON)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}
	id, _ := res.LastInsertId()
	return &Credential{ID: id, UserID: userID, CredentialID: credID, Name: name, CredentialJSON: credJSON}, nil
}

func (db *DB) ListCredentials(ctx context.Context, userID int64) ([]Credential, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT id, user_id, credential_id, name, credential_json, created_at, last_used_at
		 FROM webauthn_credentials WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()
	var creds []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.Name, &c.CredentialJSON, &c.CreatedAt, &c.LastUsedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// GetCredentialsByWebAuthnID returns all credentials for the user identified by their WebAuthn user handle.
func (db *DB) GetCredentialsByWebAuthnID(ctx context.Context, webauthnID []byte) ([]Credential, int64, error) {
	var userID int64
	err := db.sql.QueryRowContext(ctx, `SELECT id FROM users WHERE webauthn_id = ?`, webauthnID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("lookup user by webauthn_id: %w", err)
	}
	creds, err := db.ListCredentials(ctx, userID)
	return creds, userID, err
}

func (db *DB) GetCredential(ctx context.Context, id, userID int64) (*Credential, error) {
	var c Credential
	err := db.sql.QueryRowContext(ctx,
		`SELECT id, user_id, credential_id, name, credential_json, created_at, last_used_at
		 FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&c.ID, &c.UserID, &c.CredentialID, &c.Name, &c.CredentialJSON, &c.CreatedAt, &c.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (db *DB) RenameCredential(ctx context.Context, id, userID int64, name string) error {
	res, err := db.sql.ExecContext(ctx,
		`UPDATE webauthn_credentials SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	if err != nil {
		return fmt.Errorf("rename credential: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) UpdateCredentialUsed(ctx context.Context, credID []byte, credJSON string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE webauthn_credentials SET credential_json = ?, last_used_at = CURRENT_TIMESTAMP WHERE credential_id = ?`,
		credJSON, credID)
	return err
}

func (db *DB) DeleteCredential(ctx context.Context, id, userID int64) error {
	res, err := db.sql.ExecContext(ctx,
		`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) CountCredentials(ctx context.Context, userID int64) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

func (db *DB) CreateWebAuthnSession(ctx context.Context, sessionID, ceremony string, userID *int64, sessionJSON string, expiresAt time.Time) error {
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO webauthn_sessions (session_id, ceremony, user_id, session_json, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID, ceremony, userID, sessionJSON, expiresAt)
	return err
}

func (db *DB) GetAndDeleteWebAuthnSession(ctx context.Context, sessionID string) (*WebAuthnSession, error) {
	var s WebAuthnSession
	err := db.sql.QueryRowContext(ctx,
		`SELECT session_id, ceremony, user_id, session_json, expires_at FROM webauthn_sessions WHERE session_id = ?`,
		sessionID).Scan(&s.SessionID, &s.Ceremony, &s.UserID, &s.SessionJSON, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get webauthn session: %w", err)
	}
	_, _ = db.sql.ExecContext(ctx, `DELETE FROM webauthn_sessions WHERE session_id = ?`, sessionID)
	return &s, nil
}

func (db *DB) PruneWebAuthnSessions(ctx context.Context) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM webauthn_sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	return err
}
