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
}

const favColumns = `id, node_id, node_number, callsign, description, location, cmd, sort_order, group_name`

func scanFavorite(row interface{ Scan(...any) error }) (*Favorite, error) {
	var f Favorite
	err := row.Scan(&f.ID, &f.NodeID, &f.NodeNumber, &f.Callsign, &f.Description,
		&f.Location, &f.Cmd, &f.SortOrder, &f.GroupName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

func (db *DB) ListFavoritesByNode(ctx context.Context, nodeID int64) ([]Favorite, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT `+favColumns+` FROM favorites WHERE node_id=? ORDER BY sort_order, id`, nodeID)
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
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO favorites (node_id, node_number, callsign, description, location, cmd, sort_order, group_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.NodeID, f.NodeNumber, f.Callsign, f.Description, f.Location, f.Cmd, f.SortOrder, f.GroupName,
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
		`UPDATE favorites SET callsign=?, description=?, location=?, cmd=?, sort_order=?, group_name=?
		 WHERE id=?`,
		f.Callsign, f.Description, f.Location, f.Cmd, f.SortOrder, f.GroupName, f.ID,
	)
	return err
}

func (db *DB) DeleteFavorite(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM favorites WHERE id=?`, id)
	return err
}

func (db *DB) DeleteFavoritesByNode(ctx context.Context, nodeID int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM favorites WHERE node_id=?`, nodeID)
	return err
}
