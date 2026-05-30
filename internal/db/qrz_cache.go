package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LoadQRZCache returns all persisted QRZ records as raw JSON, keyed by callsign.
func (db *DB) LoadQRZCache(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT callsign, record_json FROM qrz_cache`)
	if err != nil {
		return nil, fmt.Errorf("load qrz cache: %w", err)
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var call, raw string
		if err := rows.Scan(&call, &raw); err != nil {
			return nil, err
		}
		out[call] = json.RawMessage(raw)
	}
	return out, rows.Err()
}

// SaveQRZRecord upserts a single QRZ record into the cache.
func (db *DB) SaveQRZRecord(ctx context.Context, callsign string, record json.RawMessage, fetchedAt time.Time) error {
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO qrz_cache (callsign, record_json, fetched_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(callsign) DO UPDATE SET
		   record_json = excluded.record_json,
		   fetched_at  = excluded.fetched_at`,
		callsign, string(record), fetchedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save qrz record %s: %w", callsign, err)
	}
	return nil
}
