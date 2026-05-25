package server

import (
	"testing"
)

func TestParseRPTALinks_OneLink(t *testing.T) {
	// Real-world output from a node with one active AllStar connection.
	output := "Value: RPT_ALINKS=1,41522TU\n"
	links := parseRPTALinks(output)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	if tc, ok := links["41522"]; !ok || tc != "T" {
		t.Errorf("expected 41522→T, got %v", links)
	}
}

func TestParseRPTALinks_NoLinks(t *testing.T) {
	output := "Value: RPT_ALINKS=0\n"
	links := parseRPTALinks(output)
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d: %v", len(links), links)
	}
}

func TestParseRPTALinks_MultipleLinks(t *testing.T) {
	output := "RPT_ALINKS=3,41522TU,27339RU,12345TK\n"
	links := parseRPTALinks(output)
	if len(links) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(links), links)
	}
	if links["41522"] != "T" {
		t.Errorf("41522: expected T, got %q", links["41522"])
	}
	if links["27339"] != "R" {
		t.Errorf("27339: expected R, got %q", links["27339"])
	}
	if links["12345"] != "T" {
		t.Errorf("12345: expected T, got %q", links["12345"])
	}
}

func TestParseRPTALinks_MarkerAbsent(t *testing.T) {
	output := "Value: RPT_LINKS=168,T41522,T27339\n"
	links := parseRPTALinks(output)
	if links != nil {
		t.Errorf("expected nil when RPT_ALINKS marker is absent, got %v", links)
	}
}

func TestParseRPTALinks_FullAMIBlock(t *testing.T) {
	// Simulate the multi-line block returned by "rpt show variables".
	output := `
Value: RPT_TXKEYED=0
Value: RPT_LINKS=1,T41522
Value: RPT_ALINKS=1,41522TU
Value: RPT_NUMALINKS=1
`
	links := parseRPTALinks(output)
	if len(links) != 1 {
		t.Fatalf("expected 1, got %d: %v", len(links), links)
	}
	if links["41522"] != "T" {
		t.Errorf("expected 41522→T, got %v", links)
	}
}

func TestParseRPTALinks_IgnoresRPTLinks(t *testing.T) {
	// RPT_LINKS lists 168 EchoLink nodes; RPT_ALINKS has only 1 real link.
	// We must parse RPT_ALINKS only.
	output := "RPT_LINKS=168,T41522\nRPT_ALINKS=1,41522TU\n"
	links := parseRPTALinks(output)
	if len(links) != 1 {
		t.Errorf("should parse exactly the 1 AllStar link, got %d: %v", len(links), links)
	}
}

func TestParseRPTALinks_EmptyString(t *testing.T) {
	if links := parseRPTALinks(""); links != nil {
		t.Errorf("expected nil for empty input, got %v", links)
	}
}

func TestParseRPTALinks_AllTypeChars(t *testing.T) {
	// Verify each type char used for Direction is preserved exactly.
	// T=transceive, M/R=monitor, L=local monitor, P=permanent transceive.
	output := "RPT_ALINKS=4,11111TU,22222MU,33333LU,44444PU\n"
	links := parseRPTALinks(output)
	cases := map[string]string{
		"11111": "T",
		"22222": "M",
		"33333": "L",
		"44444": "P",
	}
	for nodeNum, want := range cases {
		if got := links[nodeNum]; got != want {
			t.Errorf("node %s: expected type %q, got %q", nodeNum, want, got)
		}
	}
}

func TestParseRPTALinks_KeyedState(t *testing.T) {
	// Keyed state (K/U) must be stripped; only the type char returned.
	output := "RPT_ALINKS=2,55555TK,66666MU\n"
	links := parseRPTALinks(output)
	if links["55555"] != "T" {
		t.Errorf("keyed node 55555: expected T, got %q", links["55555"])
	}
	if links["66666"] != "M" {
		t.Errorf("unkeyed node 66666: expected M, got %q", links["66666"])
	}
}
