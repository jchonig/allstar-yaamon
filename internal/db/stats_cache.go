package db

import (
	"context"
	"encoding/json"
	"fmt"
)

// LoadStatsCache returns all persisted stats entries as raw JSON, keyed by node number.
func (db *DB) LoadStatsCache(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT node_number, stats_json FROM stats_cache`)
	if err != nil {
		return nil, fmt.Errorf("load stats cache: %w", err)
	}
	defer rows.Close()
	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var num, raw string
		if err := rows.Scan(&num, &raw); err != nil {
			return nil, err
		}
		out[num] = json.RawMessage(raw)
	}
	return out, rows.Err()
}

// SaveStatsCache upserts the given entries into the stats_cache table.
func (db *DB) SaveStatsCache(ctx context.Context, entries map[string]json.RawMessage) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for num, raw := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stats_cache (node_number, stats_json, updated_at)
			 VALUES (?, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(node_number) DO UPDATE SET
			   stats_json = excluded.stats_json,
			   updated_at = excluded.updated_at`,
			num, string(raw)); err != nil {
			return fmt.Errorf("save stats cache %s: %w", num, err)
		}
	}
	return tx.Commit()
}

// PruneStatsCache removes entries whose node_number does not appear in the
// nodes or favorites tables, preventing unbounded cache growth after nodes
// or favorites are deleted.
func (db *DB) PruneStatsCache(ctx context.Context) (int64, error) {
	res, err := db.sql.ExecContext(ctx, `
		DELETE FROM stats_cache
		WHERE node_number NOT IN (
			SELECT node_number FROM nodes
			UNION
			SELECT node_number FROM favorites
		)`)
	if err != nil {
		return 0, fmt.Errorf("prune stats cache: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
