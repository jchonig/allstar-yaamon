package astdb

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `2000|WB6NIL|ASL Public Hub|Los Angeles, CA
2001|WB6NIL|ASL Public Hub|Los Angeles, CA
2002|WB6NIL|AllStarLink Parrot|AWS US-EAST-1
28500|W5ALC|146.52|Oklahoma City, OK
99999|N0CALL|Test Node|
`
	nodes, err := parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 5 {
		t.Errorf("got %d nodes, want 5", len(nodes))
	}

	n, ok := nodes["2000"]
	if !ok {
		t.Fatal("node 2000 not found")
	}
	if n.Callsign != "WB6NIL" {
		t.Errorf("callsign = %q, want WB6NIL", n.Callsign)
	}
	if n.Description != "ASL Public Hub" {
		t.Errorf("description = %q, want ASL Public Hub", n.Description)
	}
	if n.Location != "Los Angeles, CA" {
		t.Errorf("location = %q, want Los Angeles, CA", n.Location)
	}

	n2, ok := nodes["28500"]
	if !ok {
		t.Fatal("node 28500 not found")
	}
	if n2.Callsign != "W5ALC" || n2.Description != "146.52" || n2.Location != "Oklahoma City, OK" {
		t.Errorf("28500 = %+v, want W5ALC/146.52/Oklahoma City, OK", n2)
	}

	// Node with missing location field
	n3 := nodes["99999"]
	if n3.Callsign != "N0CALL" {
		t.Errorf("99999 callsign = %q, want N0CALL", n3.Callsign)
	}
	if n3.Location != "" {
		t.Errorf("99999 location = %q, want empty", n3.Location)
	}
}

func TestParseEmptyLines(t *testing.T) {
	input := "\n\n2000|WB6NIL|Hub|\n\n"
	nodes, err := parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("got %d nodes, want 1", len(nodes))
	}
}

func TestLookup(t *testing.T) {
	db := New()
	db.mu.Lock()
	db.nodes["12345"] = Node{Callsign: "W1AW", Description: "ARRL HQ", Location: "Newington, CT"}
	db.mu.Unlock()

	n, ok := db.Lookup("12345")
	if !ok {
		t.Fatal("Lookup(12345) not found")
	}
	if n.Callsign != "W1AW" {
		t.Errorf("Callsign = %q, want W1AW", n.Callsign)
	}

	_, ok = db.Lookup("99999")
	if ok {
		t.Error("Lookup(99999) should not be found")
	}
}
