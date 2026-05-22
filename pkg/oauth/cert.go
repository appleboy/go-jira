package oauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

// loopbackCertTTL bounds the validity of a generated callback certificate. A
// fresh cert is minted on every login, so the window only needs to comfortably
// outlast one interactive authorization; it is kept short to limit the lifetime
// of the throwaway key.
const loopbackCertTTL = 24 * time.Hour

// loopbackClockSkew backdates NotBefore by a small margin so a modest
// client/server clock skew cannot reject an otherwise-valid cert. It is kept
// far smaller than loopbackCertTTL so the effective validity window stays close
// to loopbackCertTTL rather than doubling it.
const loopbackClockSkew = 5 * time.Minute

// GenerateLoopbackCert mints a self-signed certificate (with its private key)
// for the loopback callback server, entirely in memory — nothing is written to
// disk or baked into the binary.
//
// It exists so the https callback (which Jira DC requires, since it commonly
// rejects an http redirect URI) works with zero setup: the user does not need
// mkcert or any pre-provisioned cert/key files. The trade-off is that the cert
// is not signed by a CA in the system trust store, so the browser shows a
// one-time security warning the user must accept; on 127.0.0.1 this is a single
// "proceed anyway" click.
//
// The certificate template follows the conventions a tool like mkcert uses for
// a leaf cert: a random serial, SANs covering both the loopback IP literals and
// "localhost", and the ServerAuth extended key usage browsers require.
func GenerateLoopbackCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("oauth: generate loopback key: %w", err)
	}

	// A random 128-bit serial (rather than a fixed value) matches mkcert and the
	// CA/Browser Forum guidance, so two concurrently generated certs never share
	// a serial.
	// rand.Int returns a value in [0, max); X.509 serials must be positive, so
	// sample from [1, max) by adding one to a [0, max-1) draw.
	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, new(big.Int).Sub(serialMax, big.NewInt(1)))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("oauth: generate serial: %w", err)
	}
	serial.Add(serial, big.NewInt(1))

	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: loopbackHost},
		NotBefore:    now.Add(-loopbackClockSkew),
		NotAfter:     now.Add(loopbackCertTTL),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// The callback server binds 127.0.0.1, but cover ::1 and localhost too so
		// the same cert stays valid if the loopback host ever broadens.
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("oauth: create loopback certificate: %w", err)
	}

	// Parse the DER we just produced so Leaf is a real parsed certificate (with
	// Raw and friends populated), not the template struct — the latter would
	// mislead any code that later inspects cert.Leaf.
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("oauth: parse loopback certificate: %w", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}
