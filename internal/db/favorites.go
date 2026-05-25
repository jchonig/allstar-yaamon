package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Favorite struct {
	ID          int64
	NodeID      int64
	NodeNumber  string
	Callsign    string
	Description string
	Location    string
	Cmd         string
	SortOrder   int
	GroupName   string
	Position    int
}

const favColumns = `id, node_id, node_number, callsign, description, location, cmd, sort_order, group_name, position`

func scanFavorite(row interface{ Scan(...any) error }) (*Favorite, error) {
	var f Favorite
	err := row.Scan(&f.ID, &f.NodeID, &f.NodeNumber, &f.Callsign, &f.Description,
		&f.Location, &f.Cmd, &f.SortOrder, &f.GroupName, &f.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

func (db *DB) ListFavoritesByNode(ctx context.Context, nodeID int64) ([]Favorite, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT `+favColumns+` FROM favorites WHERE node_id=? ORDER BY position, id`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()
	var favs []Favorite
	for rows.Next() {
		f, err := scanFavorite(rows)
		if err != nil {
			return nil, err
		}
		favs = append(favs, *f)
	}
	return favs, rows.Err()
}

func (db *DB) GetFavoriteByNodeNumber(ctx context.Context, nodeID int64, nodeNumber string) (*Favorite, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+favColumns+` FROM favorites WHERE node_id=? AND node_number=?`, nodeID, nodeNumber)
	f, err := scanFavorite(row)
	if err != nil {
		return nil, fmt.Errorf("get favorite %s: %w", nodeNumber, err)
	}
	return f, nil
}

func (db *DB) CreateFavorite(ctx context.Context, f Favorite) (*Favorite, error) {
	// position defaults to a large value so new favorites sort to the end.
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO favorites (node_id, node_number, callsign, description, location, cmd, sort_order, group_name, position)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM favorites WHERE node_id=?), 0))`,
		f.NodeID, f.NodeNumber, f.Callsign, f.Description, f.Location, f.Cmd, f.SortOrder, f.GroupName, f.NodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("create favorite: %w", err)
	}
	id, _ := res.LastInsertId()
	f.ID = id
	return &f, nil
}

func (db *DB) UpdateFavorite(ctx context.Context, f Favorite) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE favorites SET node_number=?, callsign=?, description=?, location=?, cmd=?, sort_order=?, group_name=?
		 WHERE id=?`,
		f.NodeNumber, f.Callsign, f.Description, f.Location, f.Cmd, f.SortOrder, f.GroupName, f.ID,
	)
	return err
}

// ReorderFavorites sets the position of each favorite ID in order, within a transaction.
func (db *DB) ReorderFavorites(ctx context.Context, nodeID int64, orderedIDs []int64) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, `UPDATE favorites SET position=? WHERE id=? AND node_id=?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range orderedIDs {
		if _, err := stmt.ExecContext(ctx, i, id, nodeID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CopyFavorites copies the selected favorites (by ID) from srcNodeID to dstNodeID,
// skipping any node_number that already exists on the destination.
func (db *DB) CopyFavorites(ctx context.Context, srcNodeID, dstNodeID int64, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// Build a set of existing node_numbers on the destination to avoid duplicates.
	existing := make(map[string]struct{})
	rows, err := db.sql.QueryContext(ctx, `SELECT node_number FROM favorites WHERE node_id=?`, dstNodeID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			existing[n] = struct{}{}
		}
	}
	rows.Close()

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	copied := 0
	for _, id := range ids {
		var f Favorite
		row := tx.QueryRowContext(ctx,
			`SELECT `+favColumns+` FROM favorites WHERE id=? AND node_id=?`, id, srcNodeID)
		if err := row.Scan(&f.ID, &f.NodeID, &f.NodeNumber, &f.Callsign, &f.Description,
			&f.Location, &f.Cmd, &f.SortOrder, &f.GroupName, &f.Position); err != nil {
			continue
		}
		if _, ok := existing[f.NodeNumber]; ok {
			continue
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO favorites (node_id, node_number, callsign, description, location, cmd, sort_order, group_name, position)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT MAX(position)+1 FROM favorites WHERE node_id=?), 0))`,
			dstNodeID, f.NodeNumber, f.Callsign, f.Description, f.Location, f.Cmd, f.SortOrder, f.GroupName, dstNodeID,
		)
		if err != nil {
			return copied, err
		}
		existing[f.NodeNumber] = struct{}{}
		copied++
	}
	return copied, tx.Commit()
}

func (db *DB) DeleteFavorite(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM favorites WHERE id=?`, id)
	return err
}

func (db *DB) DeleteFavoritesByNode(ctx context.Context, nodeID int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM favorites WHERE node_id=?`, nodeID)
	return err
}
