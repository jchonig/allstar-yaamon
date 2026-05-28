package mdns

import (
	"net"
	"testing"
)

func TestIsLocal(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"allstar1.local", true},
		{"ALLSTAR1.LOCAL", true},
		{"allstar1.Local", true},
		{"allstar1.local.", true},
		{"allstar1", false},
		{"allstar1.example.com", false},
		{"localhost", false},
		{"192.168.1.1", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsLocal(tc.host); got != tc.want {
			t.Errorf("IsLocal(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestBuildQuery(t *testing.T) {
	pkt, err := buildQuery("allstar1.local")
	if err != nil {
		t.Fatalf("buildQuery: %v", err)
	}
	if len(pkt) < 13 {
		t.Fatalf("packet too short: %d bytes", len(pkt))
	}
	// QDCOUNT must be 1 (header bytes 4-5).
	if pkt[4] != 0x00 || pkt[5] != 0x01 {
		t.Errorf("QDCOUNT = %d, want 1", int(pkt[4])<<8|int(pkt[5]))
	}
	// QU bit must be set in QCLASS (bit 15 of last two bytes).
	if pkt[len(pkt)-2]&0x80 == 0 {
		t.Errorf("QU bit not set (QCLASS byte = 0x%02x)", pkt[len(pkt)-2])
	}
	// QTYPE must be A (0x0001), two bytes before QCLASS.
	qtype := int(pkt[len(pkt)-4])<<8 | int(pkt[len(pkt)-3])
	if qtype != 1 {
		t.Errorf("QTYPE = %d, want 1 (A)", qtype)
	}
}

func TestParseResponse(t *testing.T) {
	wantIP := net.IP{192, 168, 1, 10}
	pkt := buildFakeResponse(t, "test.local.", wantIP)

	ip := parseResponse(pkt, "test.local.")
	if ip == nil {
		t.Fatal("parseResponse returned nil, want IP")
	}
	if !ip.Equal(wantIP) {
		t.Errorf("parseResponse = %v, want %v", ip, wantIP)
	}
}

func TestParseResponse_WrongName(t *testing.T) {
	pkt := buildFakeResponse(t, "other.local.", net.IP{10, 0, 0, 1})
	if ip := parseResponse(pkt, "test.local."); ip != nil {
		t.Errorf("parseResponse for wrong name = %v, want nil", ip)
	}
}

func TestParseResponse_InvalidPacket(t *testing.T) {
	if ip := parseResponse([]byte{0xff, 0xfe, 0xfd}, "test.local."); ip != nil {
		t.Errorf("parseResponse for garbage = %v, want nil", ip)
	}
}

// buildFakeResponse constructs a minimal DNS response packet with a single
// A record, suitable for testing parseResponse without network access.
// It builds from the corresponding query packet so that name compression
// pointers are valid.
func buildFakeResponse(t *testing.T, fqdn string, ip net.IP) []byte {
	t.Helper()
	base, err := buildQuery(fqdn)
	if err != nil {
		t.Fatalf("buildFakeResponse: build query: %v", err)
	}
	// Flip QR bit (make it a response) and set ANCOUNT = 1.
	base[2] |= 0x80        // QR = 1
	base[6], base[7] = 0, 1 // ANCOUNT = 1

	// Answer RR: name pointer to offset 12 (start of question name),
	// Type=A, Class=IN, TTL=120, RDLENGTH=4, RDATA=ip.
	answer := []byte{
		0xc0, 0x0c, // Name: pointer to offset 12
		0x00, 0x01, // Type: A
		0x00, 0x01, // Class: IN
		0x00, 0x00, 0x00, 0x78, // TTL: 120
		0x00, 0x04, // RDLENGTH: 4
		ip[0], ip[1], ip[2], ip[3],
	}
	return append(base, answer...)
}
