package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetUserByTailscaleLogin returns the user whose tailscale_logins entry matches login, or ErrNotFound.
func (db *DB) GetUserByTailscaleLogin(ctx context.Context, login string) (*User, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+userSelectCols+` FROM users u
		 JOIN tailscale_logins tl ON tl.user_id = u.id
		 WHERE tl.login = ?`, login)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by tailscale login: %w", err)
	}
	return u, nil
}

// GetTailscaleLogins returns the Tailscale logins assigned to a user, sorted.
func (db *DB) GetTailscaleLogins(ctx context.Context, userID int64) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT login FROM tailscale_logins WHERE user_id = ? ORDER BY login`, userID)
	if err != nil {
		return nil, fmt.Errorf("get tailscale logins: %w", err)
	}
	defer rows.Close()
	var logins []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		logins = append(logins, login)
	}
	return logins, rows.Err()
}

// SetTailscaleLogins replaces all Tailscale logins for a user atomically.
// The PRIMARY KEY on tailscale_logins.login enforces uniqueness across users.
func (db *DB) SetTailscaleLogins(ctx context.Context, userID int64, logins []string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tailscale_logins WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("clear tailscale logins: %w", err)
	}
	for _, login := range logins {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tailscale_logins (login, user_id) VALUES (?, ?)`, login, userID); err != nil {
			return fmt.Errorf("tailscale login %q is already assigned to another account", login)
		}
	}
	return tx.Commit()
}
