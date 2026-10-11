package gorums_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// tlsPeerContext returns a context whose peer presented cert over TLS. If
// verified is true, cert also starts a verified chain, as after a handshake
// that checked it against a trusted CA.
func tlsPeerContext(cert *x509.Certificate, verified bool) context.Context {
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	if verified {
		state.VerifiedChains = [][]*x509.Certificate{{cert}}
	}
	return peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: state}})
}

// otherAuthInfo is the AuthInfo of a connection that does not use TLS.
type otherAuthInfo struct{}

func (otherAuthInfo) AuthType() string { return "other" }

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", s, err)
	}
	return u
}

// TestPeerVerifierTLS verifies that TLSPeerVerifier accepts a stream only if
// the verified certificate has a SAN of the type that the node's configured
// identity names, equal to that identity, and that it ignores wildcard DNS
// SANs, IP SANs, the Common Name, and unverified certificates.
func TestPeerVerifierTLS(t *testing.T) {
	node2 := mustParseURL(t, "spiffe://relab/node/2")
	node3 := mustParseURL(t, "spiffe://relab/node/3")
	node4 := mustParseURL(t, "node4.relab.example")
	node9 := mustParseURL(t, "spiffe://relab/node/A")
	node10 := mustParseURL(t, "spiffe://relab/node/10")
	verify := gorums.TLSPeerVerifier(map[gorums.ID]string{
		2:  "spiffe://relab/node/2",
		4:  "node4.relab.example",
		5:  "*.relab.example",
		6:  "10.0.0.6",
		7:  "node7.relab.example",
		8:  "",
		9:  "spiffe://relab/node/a",
		10: "SPIFFE://relab/node/10",
	})
	tests := []struct {
		name    string
		ctx     context.Context
		id      gorums.ID
		wantErr string // empty means the stream is accepted
	}{
		{name: "URIMatch", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, true), id: 2},
		{name: "OtherNodesCertificate", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node3}}, true), id: 2, wantErr: `no URI SAN equal to "spiffe://relab/node/2"`},
		{name: "DNSMatchIgnoresCase", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{"Node4.Relab.Example"}}, true), id: 4},
		{name: "WildcardDNSNeverMatches", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{"*.relab.example"}}, true), id: 5, wantErr: `no DNS SAN equal to "*.relab.example"`},
		{name: "IPSANIgnored", ctx: tlsPeerContext(&x509.Certificate{IPAddresses: []net.IP{net.ParseIP("10.0.0.6")}}, true), id: 6, wantErr: `no DNS SAN equal to "10.0.0.6"`},
		{name: "CommonNameIgnored", ctx: tlsPeerContext(&x509.Certificate{Subject: pkix.Name{CommonName: "node7.relab.example"}}, true), id: 7, wantErr: `no DNS SAN equal to "node7.relab.example"`},
		{name: "UnverifiedCertificate", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, false), id: 2, wantErr: "no verified certificate"},
		{name: "NotTLS", ctx: peer.NewContext(context.Background(), &peer.Peer{AuthInfo: otherAuthInfo{}}), id: 2, wantErr: "connection does not use TLS"},
		{name: "NoPeer", ctx: context.Background(), id: 2, wantErr: "no peer information in the stream context"},
		{name: "NoIdentity", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node3}}, true), id: 3, wantErr: "no identity configured for node 3"},
		{name: "EmptyIdentity", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{""}}, true), id: 8, wantErr: "no identity configured for node 8"},
		{name: "DNSSANDoesNotMatchURIIdentity", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{"SPIFFE://RELAB/NODE/2"}}, true), id: 2, wantErr: `no URI SAN equal to "spiffe://relab/node/2"`},
		{name: "URISANDoesNotMatchDNSIdentity", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node4}}, true), id: 4, wantErr: `no DNS SAN equal to "node4.relab.example"`},
		{name: "URIMatchIsCaseSensitive", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node9}}, true), id: 9, wantErr: `no URI SAN equal to "spiffe://relab/node/a"`},
		{name: "LaterSANMatches", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node3, node2}}, true), id: 2},
		{name: "URIIdentityCanonicalized", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node10}}, true), id: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verify(tt.ctx, tt.id)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("verify(node %d) = %v; want nil", tt.id, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("verify(node %d) = %v; want an error containing %q", tt.id, err, tt.wantErr)
			}
		})
	}

	t.Run("MapChangedAfterConstruction", func(t *testing.T) {
		identities := map[gorums.ID]string{2: "spiffe://relab/node/2"}
		verify := gorums.TLSPeerVerifier(identities)
		identities[2] = "spiffe://relab/node/3"
		if err := verify(tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, true), 2); err != nil {
			t.Errorf("verify(node 2) after the caller changed its map = %v; want nil", err)
		}
	})
}

// TestPeerVerifierTLSDuplicateIdentity verifies that TLSPeerVerifier panics
// when one SAN would match two node IDs, naming the lowest such pair, and
// accepts identities that no single SAN matches together.
func TestPeerVerifierTLSDuplicateIdentity(t *testing.T) {
	tests := []struct {
		name       string
		identities map[gorums.ID]string
		wantPanic  string // empty means no panic
	}{
		{
			name:       "DuplicateURI",
			identities: map[gorums.ID]string{1: "spiffe://relab/node/1", 2: "spiffe://relab/node/2", 3: "spiffe://relab/node/1"},
			wantPanic:  `gorums: TLSPeerVerifier: nodes 1 and 3 have the same identity "spiffe://relab/node/1"`,
		},
		{
			name:       "DuplicateURICanonicalForm",
			identities: map[gorums.ID]string{4: "SPIFFE://relab/node/4", 5: "spiffe://relab/node/4"},
			wantPanic:  `gorums: TLSPeerVerifier: nodes 4 and 5 have the same identity "spiffe://relab/node/4"`,
		},
		{
			name:       "DuplicateDNSIgnoringCase",
			identities: map[gorums.ID]string{2: "node2.relab.example", 7: "Node2.Relab.Example"},
			wantPanic:  `gorums: TLSPeerVerifier: nodes 2 and 7 have the same identity "node2.relab.example"`,
		},
		{
			// A URI SAN matches only URI identities and a DNS SAN only DNS
			// identities, so no SAN matches both nodes. The first string
			// starts with U+017F (LATIN SMALL LETTER LONG S), so url.Parse
			// rejects it and it is a DNS identity, yet strings.EqualFold
			// treats it as equal to the URI identity.
			name:       "URIAndDNSIdentityEqualFold",
			identities: map[gorums.ID]string{1: "ſpiffe://relab/node/1", 2: "spiffe://relab/node/1"},
		},
		{
			// URI SANs match exactly, so a SAN matches at most one of them.
			name:       "URIsDifferingInCase",
			identities: map[gorums.ID]string{1: "spiffe://relab/node/a", 2: "SPIFFE://RELAB/NODE/A"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic != "" {
				assertPanic(t, tt.wantPanic, func() { gorums.TLSPeerVerifier(tt.identities) })
				return
			}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("TLSPeerVerifier(%v) panicked: %v; want no panic", tt.identities, r)
				}
			}()
			gorums.TLSPeerVerifier(tt.identities)
		})
	}
}
