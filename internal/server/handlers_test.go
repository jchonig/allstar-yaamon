package server

import (
	"context"
	"testing"
	"time"

	"allstar-yaamon/internal/astdb"
	"allstar-yaamon/internal/db"
)

// stubAstDB satisfies nodeDBer for testing fillNodeInfo without a real astdb.DB.
type stubAstDB struct {
	nodes map[string]astdb.Node
}

func (s *stubAstDB) Lookup(num string) (astdb.Node, bool) {
	n, ok := s.nodes[num]
	return n, ok
}

func (s *stubAstDB) Start(_ context.Context, _ time.Duration) {}

var _ nodeDBer = (*stubAstDB)(nil) // compile-time interface check

func TestFillNodeInfo_FallsBackToAstDB(t *testing.T) {
	stub := &stubAstDB{nodes: map[string]astdb.Node{
		"12345": {Description: "146.520 simplex", Location: "Anytown, ST"},
	}}
	s := &Server{nodeDB: stub}

	nodes := []db.Node{
		{ID: 1, NodeNumber: "12345", Name: "Home"},
	}
	s.fillNodeInfo(nodes)

	if nodes[0].Description != "146.520 simplex" {
		t.Errorf("Description = %q, want %q", nodes[0].Description, "146.520 simplex")
	}
	if nodes[0].Location != "Anytown, ST" {
		t.Errorf("Location = %q, want %q", nodes[0].Location, "Anytown, ST")
	}
}

func TestFillNodeInfo_PreservesExistingDescription(t *testing.T) {
	stub := &stubAstDB{nodes: map[string]astdb.Node{
		"12345": {Description: "astdb description", Location: "astdb location"},
	}}
	s := &Server{nodeDB: stub}

	nodes := []db.Node{
		{ID: 1, NodeNumber: "12345", Name: "Home", Description: "manual desc", Location: "manual loc"},
	}
	s.fillNodeInfo(nodes)

	if nodes[0].Description != "manual desc" {
		t.Errorf("Description overwritten: got %q, want %q", nodes[0].Description, "manual desc")
	}
	if nodes[0].Location != "manual loc" {
		t.Errorf("Location overwritten: got %q, want %q", nodes[0].Location, "manual loc")
	}
}

func TestFillNodeInfo_FallsBackPartially(t *testing.T) {
	// Description set manually, Location empty → only Location filled from astdb.
	stub := &stubAstDB{nodes: map[string]astdb.Node{
		"12345": {Description: "astdb desc", Location: "astdb loc"},
	}}
	s := &Server{nodeDB: stub}

	nodes := []db.Node{
		{ID: 1, NodeNumber: "12345", Name: "Home", Description: "manual desc"},
	}
	s.fillNodeInfo(nodes)

	if nodes[0].Description != "manual desc" {
		t.Errorf("Description should be preserved, got %q", nodes[0].Description)
	}
	if nodes[0].Location != "astdb loc" {
		t.Errorf("Location should come from astdb, got %q", nodes[0].Location)
	}
}

func TestFillNodeInfo_UnknownNodeUnchanged(t *testing.T) {
	stub := &stubAstDB{nodes: map[string]astdb.Node{}}
	s := &Server{nodeDB: stub}

	nodes := []db.Node{
		{ID: 1, NodeNumber: "99999", Name: "Unknown"},
	}
	s.fillNodeInfo(nodes)

	if nodes[0].Description != "" || nodes[0].Location != "" {
		t.Errorf("unknown node should have empty desc/loc, got %q / %q",
			nodes[0].Description, nodes[0].Location)
	}
}

func TestFillNodeInfo_MultipleNodes(t *testing.T) {
	stub := &stubAstDB{nodes: map[string]astdb.Node{
		"11111": {Description: "desc A", Location: "loc A"},
		"22222": {Description: "desc B", Location: "loc B"},
	}}
	s := &Server{nodeDB: stub}

	nodes := []db.Node{
		{ID: 1, NodeNumber: "11111"},
		{ID: 2, NodeNumber: "22222", Description: "already set"},
		{ID: 3, NodeNumber: "33333"},
	}
	s.fillNodeInfo(nodes)

	if nodes[0].Description != "desc A" {
		t.Errorf("node 0 Description = %q, want desc A", nodes[0].Description)
	}
	if nodes[1].Description != "already set" {
		t.Errorf("node 1 Description overwritten to %q", nodes[1].Description)
	}
	if nodes[1].Location != "loc B" {
		t.Errorf("node 1 Location = %q, want loc B", nodes[1].Location)
	}
	if nodes[2].Description != "" {
		t.Errorf("node 2 (unknown) Description should be empty, got %q", nodes[2].Description)
	}
}
