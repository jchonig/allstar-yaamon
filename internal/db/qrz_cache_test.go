package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSaveAndLoadQRZCache(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	rec := json.RawMessage(`{"callsign":"W1AW","first_name":"Hiram","last_name":"Maxim","state":"CT"}`)
	now := time.Now().UTC().Truncate(time.Second)

	if err := db.SaveQRZRecord(ctx, "W1AW", rec, now); err != nil {
		t.Fatalf("SaveQRZRecord: %v", err)
	}

	got, err := db.LoadQRZCache(ctx)
	if err != nil {
		t.Fatalf("LoadQRZCache: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	var result map[string]any
	if err := json.Unmarshal(got["W1AW"], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["state"] != "CT" {
		t.Errorf("state = %v, want CT", result["state"])
	}
}

func TestSaveQRZRecord_Upsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	first := json.RawMessage(`{"callsign":"W1AW","first_name":"Old"}`)
	if err := db.SaveQRZRecord(ctx, "W1AW", first, now); err != nil {
		t.Fatalf("first save: %v", err)
	}

	second := json.RawMessage(`{"callsign":"W1AW","first_name":"New"}`)
	if err := db.SaveQRZRecord(ctx, "W1AW", second, now); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.LoadQRZCache(ctx)
	if err != nil {
		t.Fatalf("LoadQRZCache: %v", err)
	}
	var result map[string]any
	json.Unmarshal(got["W1AW"], &result)
	if result["first_name"] != "New" {
		t.Errorf("expected updated first_name New, got %v", result["first_name"])
	}
}

func TestLoadQRZCache_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.LoadQRZCache(ctx)
	if err != nil {
		t.Fatalf("LoadQRZCache on empty DB: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}
