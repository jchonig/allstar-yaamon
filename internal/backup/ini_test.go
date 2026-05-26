package backup

import (
	"bytes"
	"strings"
	"testing"

	"allstar-yaamon/internal/db"
)

func TestParseINI_SectionHeader(t *testing.T) {
	ini := `
[node_12345]
[node_67890]
`
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatalf("ParseINI: %v", err)
	}
	if len(favs) != 2 {
		t.Fatalf("expected 2 favorites, got %d", len(favs))
	}
	if favs[0].NodeNumber != "12345" {
		t.Errorf("expected 12345, got %q", favs[0].NodeNumber)
	}
	if favs[1].NodeNumber != "67890" {
		t.Errorf("expected 67890, got %q", favs[1].NodeNumber)
	}
}

func TestParseINI_BareSection(t *testing.T) {
	// AllScan format uses bare [12345] without "node_" prefix.
	ini := "[41522]\n[27339]\n"
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatal(err)
	}
	if len(favs) != 2 {
		t.Fatalf("expected 2, got %d", len(favs))
	}
	if favs[0].NodeNumber != "41522" {
		t.Errorf("got %q", favs[0].NodeNumber)
	}
}

func TestParseINI_CmdLine(t *testing.T) {
	// YAAMon-exported format uses cmd[] lines.
	ini := `[node_99999]
cmd[] = "rpt cmd 99999 ilink 3 41522"
`
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatal(err)
	}
	// Should capture both the section header node (99999) and cmd target (41522).
	nums := make(map[string]bool)
	for _, f := range favs {
		nums[f.NodeNumber] = true
	}
	if !nums["99999"] {
		t.Error("expected 99999 from section header")
	}
	if !nums["41522"] {
		t.Error("expected 41522 from cmd[] line")
	}
}

func TestParseINI_Deduplication(t *testing.T) {
	ini := "[node_12345]\n[node_12345]\n[12345]\n"
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatal(err)
	}
	if len(favs) != 1 {
		t.Errorf("expected 1 after dedup, got %d", len(favs))
	}
}

func TestParseINI_IgnoresNonNodeSections(t *testing.T) {
	ini := "[general]\nkey=value\n[node_12345]\n"
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatal(err)
	}
	if len(favs) != 1 {
		t.Errorf("expected 1, got %d", len(favs))
	}
	if favs[0].NodeNumber != "12345" {
		t.Errorf("expected 12345, got %q", favs[0].NodeNumber)
	}
}

func TestParseINI_IgnoresLongNumbers(t *testing.T) {
	ini := "[node_12345678901234]\n[node_99999]\n"
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatal(err)
	}
	// Only 99999 should survive (the long one exceeds the 10-digit max).
	if len(favs) != 1 || favs[0].NodeNumber != "99999" {
		t.Errorf("got %+v", favs)
	}
}

func TestParseINI_EmptyFile(t *testing.T) {
	favs, err := ParseINI([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(favs) != 0 {
		t.Errorf("expected empty, got %d", len(favs))
	}
}

func TestExportINI_ContainsNodes(t *testing.T) {
	favs := []db.Favorite{
		{NodeNumber: "41522", Callsign: "W1AW"},
		{NodeNumber: "27339", Callsign: "N0CALL"},
	}
	out := ExportINI(favs, "99999")
	s := string(out)

	if !strings.Contains(s, "[node_41522]") {
		t.Error("missing [node_41522] section")
	}
	if !strings.Contains(s, "[node_27339]") {
		t.Error("missing [node_27339] section")
	}
	if !strings.Contains(s, "ilink 3 41522") {
		t.Error("missing ilink command for 41522")
	}
}

func TestExportImportRoundtrip(t *testing.T) {
	favs := []db.Favorite{
		{NodeNumber: "11111"},
		{NodeNumber: "22222"},
		{NodeNumber: "33333"},
	}
	exported := ExportINI(favs, "99999")
	imported, err := ParseINI(exported)
	if err != nil {
		t.Fatalf("ParseINI after export: %v", err)
	}

	nums := make(map[string]bool)
	for _, f := range imported {
		nums[f.NodeNumber] = true
	}
	for _, f := range favs {
		if !nums[f.NodeNumber] {
			t.Errorf("node %s lost in export/import round-trip", f.NodeNumber)
		}
	}
}

func TestExportINI_EmptyFavorites(t *testing.T) {
	out := ExportINI(nil, "99999")
	if !bytes.Contains(out, []byte("Home node: 99999")) {
		t.Error("expected header with home node in output")
	}
}

func TestParseINI_AllScanLabelCmd(t *testing.T) {
	ini := `[general]
label[] = "W1AW ARRL HQ, Newington CT"
cmd[] = "rpt cmd %node% ilink 3 29840"

label[] = "GB3CR North Wales UK"
cmd[] = "rpt cmd %node% ilink 3 512511"

label[] = "No callsign here"
cmd[] = "rpt cmd %node% ilink 3 11111"
`
	favs, err := ParseINI([]byte(ini))
	if err != nil {
		t.Fatalf("ParseINI: %v", err)
	}
	if len(favs) != 3 {
		t.Fatalf("expected 3 favorites, got %d: %+v", len(favs), favs)
	}

	cases := []struct{ num, cs, desc string }{
		{"29840", "W1AW", "ARRL HQ, Newington CT"},
		{"512511", "GB3CR", "North Wales UK"},
		{"11111", "", "No callsign here"},
	}
	for i, c := range cases {
		f := favs[i]
		if f.NodeNumber != c.num {
			t.Errorf("[%d] NodeNumber = %q, want %q", i, f.NodeNumber, c.num)
		}
		if f.Callsign != c.cs {
			t.Errorf("[%d] Callsign = %q, want %q", i, f.Callsign, c.cs)
		}
		if f.Description != c.desc {
			t.Errorf("[%d] Description = %q, want %q", i, f.Description, c.desc)
		}
	}
}

func TestParseLabel(t *testing.T) {
	cases := []struct{ label, cs, desc string }{
		{"W1AW ARRL HQ, Newington CT", "W1AW", "ARRL HQ, Newington CT"},
		{"K4JDR 441.725+ Backbone HUB", "K4JDR", "441.725+ Backbone HUB"},
		{"GB3CR North Wales UK", "GB3CR", "North Wales UK"},
		{"N2VLV", "N2VLV", ""},
		{"ARRL HQ", "", "ARRL HQ"},       // no digit → not a callsign
		{"", "", ""},
	}
	for _, c := range cases {
		cs, desc := parseLabel(c.label)
		if cs != c.cs || desc != c.desc {
			t.Errorf("parseLabel(%q) = (%q, %q), want (%q, %q)", c.label, cs, desc, c.cs, c.desc)
		}
	}
}

func TestParseAllmon3INI(t *testing.T) {
	ini := `
[50815]
host=172.17.16.36
user=admin
pass=secret
port=5038

[460180]
host=172.17.16.217
user=admin
pass=otherpass

[general]
; ignored section
`
	nodes, err := ParseAllmon3INI([]byte(ini))
	if err != nil {
		t.Fatalf("ParseAllmon3INI: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(nodes), nodes)
	}

	n := nodes[0]
	if n.NodeNumber != "50815" {
		t.Errorf("NodeNumber = %q", n.NodeNumber)
	}
	if n.AMIHost != "172.17.16.36" {
		t.Errorf("AMIHost = %q", n.AMIHost)
	}
	if n.AMIPort != 5038 {
		t.Errorf("AMIPort = %d", n.AMIPort)
	}
	if n.AMIUser != "admin" {
		t.Errorf("AMIUser = %q", n.AMIUser)
	}
	if n.AMIPass != "secret" {
		t.Errorf("AMIPass = %q", n.AMIPass)
	}

	// Second node should use default port when omitted
	n2 := nodes[1]
	if n2.NodeNumber != "460180" {
		t.Errorf("NodeNumber = %q", n2.NodeNumber)
	}
	if n2.AMIPort != 5038 {
		t.Errorf("AMIPort should default to 5038, got %d", n2.AMIPort)
	}
}

func TestParseAllmon3INI_Empty(t *testing.T) {
	nodes, err := ParseAllmon3INI([]byte("; just a comment\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}
