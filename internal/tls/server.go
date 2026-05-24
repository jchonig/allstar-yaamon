package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"time"

	"allstar-yaamon/internal/config"
	"golang.org/x/crypto/acme/autocert"
)

// NewTLSConfig returns a tls.Config for the given mode, or nil for "disabled".
// In Phase 1, self_signed generates a fresh cert each run; Phase 2 will persist it in SQLite.
func NewTLSConfig(cfg *config.TLSConfig) (*tls.Config, error) {
	switch cfg.Mode {
	case "disabled":
		return nil, nil

	case "provided":
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS certificate: %w", err)
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}, nil

	case "self_signed":
		cert, err := generateSelfSigned()
		if err != nil {
			return nil, fmt.Errorf("generating self-signed certificate: %w", err)
		}
		slog.Warn("using self-signed TLS certificate — browser will show a security warning")
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}, nil

	case "acme":
		m := &autocert.Manager{
			Cache:      autocert.DirCache(cfg.ACMECacheDir),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.ACMEDomain),
		}
		return m.TLSConfig(), nil

	default:
		return nil, fmt.Errorf("unknown TLS mode: %s", cfg.Mode)
	}
}

// RedirectHandler returns an http.Handler that 301-redirects all requests to HTTPS.
func RedirectHandler(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		target := "https://" + host
		if httpsPort != 443 {
			target += fmt.Sprintf(":%d", httpsPort)
		}
		target += r.RequestURI
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

func generateSelfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "yaamon"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}
