package tls

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest/certs"
	"google.golang.org/grpc/credentials"
)

// testCA is a throwaway certificate authority that issues per-replica
// certificates for the peer identity tests.
type testCA struct {
	ca   *certs.Cert
	pool *x509.CertPool
}

func newTestCA(t testing.TB) *testCA {
	t.Helper()
	ca := certs.NewCA(t)
	return &testCA{ca: ca, pool: ca.Pool()}
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
	uri, err := url.Parse(nodeIdentity(id))
	if err != nil {
		t.Fatalf("parse node identity: %v", err)
	}
	return ca.ca.Issue(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: fmt.Sprintf("node %d", id)},
		URIs:        []*url.URL{uri},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}).TLS()
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
