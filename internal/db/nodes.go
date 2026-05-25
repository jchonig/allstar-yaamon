package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Node struct {
	ID         int64
	Name       string
	NodeNumber string
	AMIHost    string
	AMIPort    int
	AMIUser    string
	AMIPass    string
	Enabled    bool
}

func scanNode(row interface{ Scan(...any) error }) (*Node, error) {
	var n Node
	var enabled int
	err := row.Scan(&n.ID, &n.Name, &n.NodeNumber, &n.AMIHost, &n.AMIPort, &n.AMIUser, &n.AMIPass, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n.Enabled = enabled != 0
	return &n, nil
}

const nodeColumns = `id, name, node_number, ami_host, ami_port, ami_user, ami_pass, enabled`

func (db *DB) GetNodeByNumber(ctx context.Context, nodeNumber string) (*Node, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE node_number = ?`, nodeNumber)
	n, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("get node %s: %w", nodeNumber, err)
	}
	return n, nil
}

func (db *DB) GetNodeByID(ctx context.Context, id int64) (*Node, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id)
	n, err := scanNode(row)
	if err != nil {
		return nil, fmt.Errorf("get node %d: %w", id, err)
	}
	return n, nil
}

func (db *DB) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

func (db *DB) CreateNode(ctx context.Context, n Node) (*Node, error) {
	enabled := 0
	if n.Enabled {
		enabled = 1
	}
	res, err := db.sql.ExecContext(ctx,
		`INSERT INTO nodes (name, node_number, ami_host, ami_port, ami_user, ami_pass, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.NodeNumber, n.AMIHost, n.AMIPort, n.AMIUser, n.AMIPass, enabled,
	)
	if err != nil {
		return nil, fmt.Errorf("create node: %w", err)
	}
	id, _ := res.LastInsertId()
	n.ID = id
	return &n, nil
}

func (db *DB) UpdateNode(ctx context.Context, n Node) error {
	enabled := 0
	if n.Enabled {
		enabled = 1
	}
	_, err := db.sql.ExecContext(ctx,
		`UPDATE nodes SET name=?, ami_host=?, ami_port=?, ami_user=?, ami_pass=?, enabled=?
		 WHERE id=?`,
		n.Name, n.AMIHost, n.AMIPort, n.AMIUser, n.AMIPass, enabled, n.ID,
	)
	return err
}

func (db *DB) DeleteNode(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM nodes WHERE id=?`, id)
	return err
}

func (db *DB) ListNodeNumbers(ctx context.Context) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT node_number FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nums []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		nums = append(nums, s)
	}
	return nums, rows.Err()
}
