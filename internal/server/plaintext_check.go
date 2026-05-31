package server

import (
	"fmt"
	"log/slog"
	"net"

	"allstar-yaamon/internal/config"
)

// privateRanges lists RFC 1918, loopback, and link-local CIDRs considered
// safe for plaintext HTTP (no public exposure).
var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"169.254.0.0/16",
		"fe80::/10",
		"fc00::/7",
	} {
		_, network, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, network)
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// checkPlaintextSafety returns an error if the server would serve plaintext
// HTTP on a public (non-RFC-1918) address. Skipped when TLS is active, when
// a proxy is handling TLS (proxy_auth or tailscale_auth enabled), or when the
// operator has explicitly set server.allow_public_plaintext: true.
func checkPlaintextSafety(cfg *config.Config) error {
	if cfg.TLS.Mode != "disabled" {
		return nil
	}
	if cfg.ProxyAuth.Enabled || cfg.TailscaleAuth.Enabled {
		return nil
	}
	if cfg.Server.AllowPublicPlaintext {
		slog.Warn("serving plaintext HTTP on potentially public addresses (allow_public_plaintext=true)")
		return nil
	}

	ips, err := resolveBindIPs(cfg.Server.BindAddress)
	if err != nil {
		return err
	}

	for _, ip := range ips {
		if !isPrivateIP(ip) {
			slog.Error("refusing to serve plaintext HTTP on a public address — credentials would be sent in the clear",
				"addr", ip.String(),
				"hint", "enable TLS (tls.mode), put YAAMon behind a TLS-terminating proxy, or set server.allow_public_plaintext: true if you understand the risk")
			return fmt.Errorf(
				"plaintext HTTP on public address %s is unsafe; enable TLS or set server.allow_public_plaintext: true",
				ip)
		}
	}
	return nil
}

// resolveBindIPs returns the IP addresses that will actually be bound.
// For wildcard addresses (empty, "0.0.0.0", "::") it enumerates all
// interface addresses so the check covers the real network exposure.
func resolveBindIPs(bindAddr string) ([]net.IP, error) {
	if bindAddr == "" || bindAddr == "0.0.0.0" || bindAddr == "::" {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return nil, fmt.Errorf("enumerating interface addresses: %w", err)
		}
		var ips []net.IP
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPNet:
				ips = append(ips, v.IP)
			case *net.IPAddr:
				ips = append(ips, v.IP)
			}
		}
		return ips, nil
	}

	ip := net.ParseIP(bindAddr)
	if ip != nil {
		return []net.IP{ip}, nil
	}

	// Hostname — resolve it.
	resolved, err := net.LookupHost(bindAddr)
	if err != nil {
		return nil, fmt.Errorf("resolving bind address %q: %w", bindAddr, err)
	}
	var ips []net.IP
	for _, r := range resolved {
		if ip := net.ParseIP(r); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}
