package tls

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
	"net/url"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc/credentials"
)

// testCA is a throwaway certificate authority that issues per-replica
// certificates for the peer identity tests.
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTestCA(t testing.TB) *testCA {
	t.Helper()
	key := newKey(t)
	tmpl := &x509.Certificate{
		SerialNumber:          newSerialNumber(t),
		Subject:               pkix.Name{CommonName: "Gorums Test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{cert: cert, key: key, pool: pool}
}

// nodeIdentity returns the URI SAN that names node id in these tests.
func nodeIdentity(id gorums.ID) string {
	return fmt.Sprintf("spiffe://gorums.test/node/%d", id)
}

// issue returns a certificate for node id with nodeIdentity(id) as its URI
// SAN, valid on the loopback addresses for both client and server
// authentication.
func (ca *testCA) issue(t testing.TB, id gorums.ID) tls.Certificate {
	t.Helper()
	key := newKey(t)
	uri, err := url.Parse(nodeIdentity(id))
	if err != nil {
		t.Fatalf("parse node identity: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: newSerialNumber(t),
		Subject:      pkix.Name{CommonName: fmt.Sprintf("node %d", id)},
		URIs:         []*url.URL{uri},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, key.Public(), ca.key)
	if err != nil {
		t.Fatalf("create certificate for node %d: %v", id, err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// serverCreds returns server credentials that present cert and verify a
// client certificate against the CA when the client sends one.
func (ca *testCA) serverCreds(cert tls.Certificate) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    ca.pool,
		ClientAuth:   tls.VerifyClientCertIfGiven,
	})
}

// dialCreds returns dial credentials that verify servers against the CA and
// present certs, if any, as the client certificate.
func (ca *testCA) dialCreds(certs ...tls.Certificate) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{
		Certificates: certs,
		RootCAs:      ca.pool,
	})
}

type peerNode struct {
	addr     string
	identity string
}

func (n peerNode) Addr() string        { return n.addr }
func (n peerNode) TLSIdentity() string { return n.identity }

func newKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func newSerialNumber(t testing.TB) *big.Int {
	t.Helper()
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial number: %v", err)
	}
	return sn
}
