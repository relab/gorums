package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// Cert is a parsed certificate and its private key.
type Cert struct {
	X509 *x509.Certificate
	Key  *ecdsa.PrivateKey
}

// New signs a certificate with a fresh key from tmpl.
// The certificate is self-signed if parent is nil, and signed by parent otherwise.
// New sets a random serial number if tmpl has none,
// and a validity period around the current time if tmpl has none.
// All other fields of tmpl are used as given.
func New(t testing.TB, tmpl *x509.Certificate, parent *Cert) *Cert {
	t.Helper()
	cp := *tmpl
	tmpl = &cp
	if tmpl.SerialNumber == nil {
		sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			t.Fatalf("generate serial number: %v", err)
		}
		tmpl.SerialNumber = sn
	}
	if tmpl.NotBefore.IsZero() && tmpl.NotAfter.IsZero() {
		tmpl.NotBefore = time.Now().Add(-time.Minute)
		tmpl.NotAfter = time.Now().Add(time.Hour)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.X509, parent.Key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, key.Public(), signerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return &Cert{X509: leaf, Key: key}
}

// Localhost returns a self-signed server certificate
// whose only SAN is the DNS name "localhost".
func Localhost(t testing.TB) *Cert {
	t.Helper()
	return New(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "localhost"},
		DNSNames:    []string{"localhost"},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, nil)
}

// Loopback returns a self-signed server certificate
// whose SANs are the IPv4 and IPv6 loopback addresses.
func Loopback(t testing.TB) *Cert {
	t.Helper()
	return New(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: "Gorums TLS Test"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, nil)
}

// NewCA returns a self-signed certificate authority.
// Use [Cert.Issue] to sign certificates with it.
func NewCA(t testing.TB) *Cert {
	t.Helper()
	return New(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "Gorums Test CA"},
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, nil)
}

// Issue signs a certificate with a fresh key from tmpl, using c as the issuer.
// See [New] for the defaults applied to tmpl.
func (c *Cert) Issue(t testing.TB, tmpl *x509.Certificate) *Cert {
	t.Helper()
	return New(t, tmpl, c)
}

// TLS returns c as a certificate for a [tls.Config].
func (c *Cert) TLS() tls.Certificate {
	return tls.Certificate{Certificate: [][]byte{c.X509.Raw}, PrivateKey: c.Key, Leaf: c.X509}
}

// Pool returns a pool that trusts c.
func (c *Cert) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.X509)
	return pool
}
