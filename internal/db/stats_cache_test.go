package db

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSaveAndLoadStatsCache(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	entries := map[string]json.RawMessage{
		"12345": json.RawMessage(`{"node_number":"12345","callsign":"W1AW","connected_links":2}`),
		"67890": json.RawMessage(`{"node_number":"67890","callsign":"K6BSY","connected_links":0}`),
	}

	if err := db.SaveStatsCache(ctx, entries); err != nil {
		t.Fatalf("SaveStatsCache: %v", err)
	}

	got, err := db.LoadStatsCache(ctx)
	if err != nil {
		t.Fatalf("LoadStatsCache: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	for num, raw := range entries {
		var want, have map[string]any
		json.Unmarshal(raw, &want)
		if err := json.Unmarshal(got[num], &have); err != nil {
			t.Errorf("node %s: unmarshal result: %v", num, err)
			continue
		}
		if have["callsign"] != want["callsign"] {
			t.Errorf("node %s: callsign = %v, want %v", num, have["callsign"], want["callsign"])
		}
	}
}

func TestSaveStatsCache_Upsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := map[string]json.RawMessage{
		"12345": json.RawMessage(`{"node_number":"12345","callsign":"W1AW"}`),
	}
	if err := db.SaveStatsCache(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	second := map[string]json.RawMessage{
		"12345": json.RawMessage(`{"node_number":"12345","callsign":"K1TTT"}`),
	}
	if err := db.SaveStatsCache(ctx, second); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.LoadStatsCache(ctx)
	if err != nil {
		t.Fatalf("LoadStatsCache: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	var result map[string]any
	json.Unmarshal(got["12345"], &result)
	if result["callsign"] != "K1TTT" {
		t.Errorf("expected updated callsign K1TTT, got %v", result["callsign"])
	}
}

func TestSaveStatsCache_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveStatsCache(ctx, nil); err != nil {
		t.Errorf("SaveStatsCache(nil): %v", err)
	}
	if err := db.SaveStatsCache(ctx, map[string]json.RawMessage{}); err != nil {
		t.Errorf("SaveStatsCache(empty): %v", err)
	}

	got, err := db.LoadStatsCache(ctx)
	if err != nil {
		t.Fatalf("LoadStatsCache: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(got))
	}
}

func TestPruneStatsCache(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Create a node and a favorite so their node numbers survive pruning.
	_, err := db.sql.ExecContext(ctx,
		`INSERT INTO nodes (name, node_number, ami_user, ami_pass) VALUES ('test', '11111', 'u', 'p')`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	var nodeID int64
	db.sql.QueryRowContext(ctx, `SELECT id FROM nodes WHERE node_number = '11111'`).Scan(&nodeID)
	db.sql.ExecContext(ctx,
		`INSERT INTO favorites (node_id, node_number) VALUES (?, '22222')`, nodeID)

	entries := map[string]json.RawMessage{
		"11111": json.RawMessage(`{"node_number":"11111"}`), // in nodes — keep
		"22222": json.RawMessage(`{"node_number":"22222"}`), // in favorites — keep
		"99999": json.RawMessage(`{"node_number":"99999"}`), // orphan — prune
	}
	if err := db.SaveStatsCache(ctx, entries); err != nil {
		t.Fatalf("SaveStatsCache: %v", err)
	}

	n, err := db.PruneStatsCache(ctx)
	if err != nil {
		t.Fatalf("PruneStatsCache: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 pruned row, got %d", n)
	}

	got, err := db.LoadStatsCache(ctx)
	if err != nil {
		t.Fatalf("LoadStatsCache after prune: %v", err)
	}
	if _, ok := got["99999"]; ok {
		t.Error("orphan entry 99999 should have been pruned")
	}
	if _, ok := got["11111"]; !ok {
		t.Error("node entry 11111 should be kept")
	}
	if _, ok := got["22222"]; !ok {
		t.Error("favorite entry 22222 should be kept")
	}
}

func TestPruneStatsCache_NothingToPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	n, err := db.PruneStatsCache(ctx)
	if err != nil {
		t.Fatalf("PruneStatsCache on empty DB: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pruned, got %d", n)
	}
}

func TestLoadStatsCache_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.LoadStatsCache(ctx)
	if err != nil {
		t.Fatalf("LoadStatsCache on empty DB: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}
