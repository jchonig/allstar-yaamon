package server

import (
	"net"
	"testing"

	"allstar-yaamon/internal/config"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		addr    string
		private bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"169.254.1.1", true},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd00::1", true},
		// public addresses
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
		{"2001:db8::1", false},
	}

	for _, tc := range tests {
		ip := net.ParseIP(tc.addr)
		if ip == nil {
			t.Fatalf("could not parse %q", tc.addr)
		}
		got := isPrivateIP(ip)
		if got != tc.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tc.addr, got, tc.private)
		}
	}
}

func TestResolveBindIPs_Specific(t *testing.T) {
	ips, err := resolveBindIPs("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || !ips[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("got %v, want [127.0.0.1]", ips)
	}
}

func TestResolveBindIPs_Wildcard(t *testing.T) {
	for _, bind := range []string{"", "0.0.0.0", "::"} {
		ips, err := resolveBindIPs(bind)
		if err != nil {
			t.Fatalf("resolveBindIPs(%q): %v", bind, err)
		}
		if len(ips) == 0 {
			t.Errorf("resolveBindIPs(%q) returned no IPs", bind)
		}
	}
}

func TestCheckPlaintextSafety_SkippedWhenTLSActive(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "self_signed"
	cfg.Server.BindAddress = "1.2.3.4" // public — but TLS is on, so no error
	if err := checkPlaintextSafety(cfg); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckPlaintextSafety_SkippedWhenProxyAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "disabled"
	cfg.ProxyAuth.Enabled = true
	cfg.Server.BindAddress = "1.2.3.4"
	if err := checkPlaintextSafety(cfg); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckPlaintextSafety_SkippedWhenTailscaleAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "disabled"
	cfg.TailscaleAuth.Enabled = true
	cfg.Server.BindAddress = "1.2.3.4"
	if err := checkPlaintextSafety(cfg); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckPlaintextSafety_PrivateAddressOK(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "disabled"
	cfg.Server.BindAddress = "192.168.1.1"
	if err := checkPlaintextSafety(cfg); err != nil {
		t.Errorf("expected nil for private address, got %v", err)
	}
}

func TestCheckPlaintextSafety_PublicAddressRefused(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "disabled"
	cfg.Server.BindAddress = "1.2.3.4"
	if err := checkPlaintextSafety(cfg); err == nil {
		t.Error("expected error for public address, got nil")
	}
}

func TestCheckPlaintextSafety_AllowPublicOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.TLS.Mode = "disabled"
	cfg.Server.BindAddress = "1.2.3.4"
	cfg.Server.AllowPublicPlaintext = true
	if err := checkPlaintextSafety(cfg); err != nil {
		t.Errorf("expected nil when allow_public_plaintext=true, got %v", err)
	}
}
