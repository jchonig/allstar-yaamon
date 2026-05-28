// Package mdns provides a minimal mDNS A-record resolver for .local hostnames.
// It sends a QU (unicast-response) query to the mDNS multicast group
// 224.0.0.251:5353 and returns the first IPv4 address in the response.
package mdns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	mdnsGroup   = "224.0.0.251"
	mdnsPort    = 5353
	mdnsTimeout = 2 * time.Second
)

// IsLocal reports whether host is a .local mDNS name (case-insensitive,
// optional trailing dot tolerated).
func IsLocal(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return strings.HasSuffix(h, ".local")
}

// Resolve looks up a .local hostname via mDNS (RFC 6762) and returns the
// first IPv4 address advertised for it.
func Resolve(ctx context.Context, host string) (net.IP, error) {
	pkt, err := buildQuery(host)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("mdns: listen: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(mdnsTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	conn.SetDeadline(deadline) //nolint:errcheck

	dst := &net.UDPAddr{IP: net.ParseIP(mdnsGroup), Port: mdnsPort}
	if _, err := conn.WriteTo(pkt, dst); err != nil {
		return nil, fmt.Errorf("mdns: send: %w", err)
	}

	fqdn := strings.ToLower(strings.TrimSuffix(host, ".")) + "."
	buf := make([]byte, 9000)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return nil, fmt.Errorf("mdns: no response for %q", host)
		}
		if ip := parseResponse(buf[:n], fqdn); ip != nil {
			return ip, nil
		}
	}
}

// buildQuery constructs a single-question mDNS A-record query for host.
// The QU bit (bit 15 of QCLASS) is set to request a unicast response,
// which avoids needing to join the multicast group to receive replies.
func buildQuery(host string) ([]byte, error) {
	fqdn := strings.TrimSuffix(host, ".") + "."
	name, err := dnsmessage.NewName(fqdn)
	if err != nil {
		return nil, fmt.Errorf("mdns: invalid hostname %q: %w", host, err)
	}
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{ID: 0, RecursionDesired: false},
		Questions: []dnsmessage.Question{{
			Name:  name,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		}},
	}
	pkt, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("mdns: pack query: %w", err)
	}
	// Set the QU bit (unicast-response request) in the QCLASS field.
	// For a single-question query with no resource records, QCLASS is
	// always in the last two bytes of the packet.
	pkt[len(pkt)-2] |= 0x80
	return pkt, nil
}

// parseResponse scans msg for an A record matching fqdn and returns the IP.
func parseResponse(msg []byte, fqdn string) net.IP {
	var m dnsmessage.Message
	if err := m.Unpack(msg); err != nil {
		return nil
	}
	for _, ans := range m.Answers {
		if ans.Header.Type != dnsmessage.TypeA {
			continue
		}
		if strings.ToLower(ans.Header.Name.String()) != fqdn {
			continue
		}
		body, ok := ans.Body.(*dnsmessage.AResource)
		if !ok {
			continue
		}
		return net.IP{body.A[0], body.A[1], body.A[2], body.A[3]}
	}
	return nil
}
