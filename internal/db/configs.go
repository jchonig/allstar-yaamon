package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetConfig returns the value for key, or "" if not set.
func (db *DB) GetConfig(ctx context.Context, key string) (string, error) {
	var val string
	err := db.sql.QueryRowContext(ctx, `SELECT value FROM configs WHERE key = ?`, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get config %q: %w", key, err)
	}
	return val, nil
}

// SetConfig inserts or updates a config key.
func (db *DB) SetConfig(ctx context.Context, key, value string) error {
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO configs (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set config %q: %w", key, err)
	}
	return nil
}
