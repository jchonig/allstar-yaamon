package server

import (
	"testing"
)

func TestParseRPTALinks_OneLink(t *testing.T) {
	output := "Value: RPT_ALINKS=1,41522TU\n"
	links := parseRPTALinks(output)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	ls, ok := links["41522"]
	if !ok {
		t.Fatalf("expected key 41522, got %v", links)
	}
	if ls.Type != "T" {
		t.Errorf("expected Type T, got %q", ls.Type)
	}
	if ls.Keyed {
		t.Errorf("expected Keyed=false for U suffix")
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
	if links["41522"].Type != "T" || links["41522"].Keyed {
		t.Errorf("41522: expected T/unkeyed, got %+v", links["41522"])
	}
	if links["27339"].Type != "R" || links["27339"].Keyed {
		t.Errorf("27339: expected R/unkeyed, got %+v", links["27339"])
	}
	if links["12345"].Type != "T" || !links["12345"].Keyed {
		t.Errorf("12345: expected T/keyed, got %+v", links["12345"])
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
	if links["41522"].Type != "T" {
		t.Errorf("expected 41522→T, got %v", links["41522"])
	}
}

func TestParseRPTALinks_IgnoresRPTLinks(t *testing.T) {
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
	output := "RPT_ALINKS=4,11111TU,22222MU,33333LU,44444PU\n"
	links := parseRPTALinks(output)
	cases := map[string]string{
		"11111": "T",
		"22222": "M",
		"33333": "L",
		"44444": "P",
	}
	for nodeNum, want := range cases {
		if got := links[nodeNum].Type; got != want {
			t.Errorf("node %s: expected type %q, got %q", nodeNum, want, got)
		}
	}
}

func TestParseRPTALinks_KeyedState(t *testing.T) {
	output := "RPT_ALINKS=2,55555TK,66666MU\n"
	links := parseRPTALinks(output)
	if !links["55555"].Keyed {
		t.Errorf("node 55555 (K suffix): expected Keyed=true")
	}
	if links["66666"].Keyed {
		t.Errorf("node 66666 (U suffix): expected Keyed=false")
	}
}

func TestParseRPTALinks_CallsignNodeIdent(t *testing.T) {
	// Direct/IAXRPT clients appear with a callsign instead of a numeric node number.
	output := "RPT_ALINKS=2,667342TU,KR4YXXTK\n"
	links := parseRPTALinks(output)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %v", len(links), links)
	}
	if links["667342"].Type != "T" || links["667342"].Keyed {
		t.Errorf("667342: expected T/unkeyed, got %+v", links["667342"])
	}
	if links["KR4YXX"].Type != "T" || !links["KR4YXX"].Keyed {
		t.Errorf("KR4YXX: expected T/keyed, got %+v", links["KR4YXX"])
	}
}

func TestParseRPTTXKeyed_Keyed(t *testing.T) {
	output := "Value: RPT_TXKEYED=1\n"
	if !parseRPTTXKeyed(output) {
		t.Error("expected true when RPT_TXKEYED=1")
	}
}

func TestParseRPTTXKeyed_NotKeyed(t *testing.T) {
	output := "Value: RPT_TXKEYED=0\n"
	if parseRPTTXKeyed(output) {
		t.Error("expected false when RPT_TXKEYED=0")
	}
}

func TestParseRPTTXKeyed_Absent(t *testing.T) {
	output := "Value: RPT_ALINKS=1,41522TU\n"
	if parseRPTTXKeyed(output) {
		t.Error("expected false when RPT_TXKEYED is absent")
	}
}

func TestParseRPTTXKeyed_FullBlock(t *testing.T) {
	output := `
Value: RPT_TXKEYED=1
Value: RPT_ALINKS=1,41522TU
`
	if !parseRPTTXKeyed(output) {
		t.Error("expected true in full AMI block with RPT_TXKEYED=1")
	}
}
