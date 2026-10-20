package conn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
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

type tlsNode struct {
	addr     string
	identity string
}

func (n tlsNode) Addr() string        { return n.addr }
func (n tlsNode) TLSIdentity() string { return n.identity }

// identityNodes constructs a source that keeps each identity with its address.
func identityNodes(identities map[ID]string) NodeSource {
	nodes := make(map[ID]tlsNode, len(identities))
	for id, identity := range identities {
		nodes[id] = tlsNode{fmt.Sprintf("127.0.0.1:%d", 9000+id), identity}
	}
	return WithNodes(nodes)
}

// TestPeerVerifierTLS verifies that the verifier accepts a stream only if
// the verified certificate has a SAN of the type that the node's configured
// identity names, equal to that identity, and that it ignores wildcard DNS
// SANs, IP SANs, the Common Name, and unverified certificates.
func TestPeerVerifierTLS(t *testing.T) {
	node2 := mustParseURL(t, "spiffe://relab/node/2")
	node3 := mustParseURL(t, "spiffe://relab/node/3")
	node4 := mustParseURL(t, "node4.relab.example")
	node9 := mustParseURL(t, "spiffe://relab/node/A")
	node10 := mustParseURL(t, "spiffe://relab/node/10")
	verify, err := NewPeerVerifier(identityNodes(map[ID]string{
		2:  "spiffe://relab/node/2",
		4:  "node4.relab.example",
		5:  "node5.relab.example",
		6:  "node6.relab.example",
		7:  "node7.relab.example",
		9:  "spiffe://relab/node/a",
		10: "SPIFFE://relab/node/10",
	}))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		ctx     context.Context
		id      ID
		wantErr string // empty means the stream is accepted
	}{
		{name: "URIMatch", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, true), id: 2},
		{name: "OtherNodesCertificate", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node3}}, true), id: 2, wantErr: `no URI SAN equal to "spiffe://relab/node/2"`},
		{name: "DNSMatchIgnoresCase", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{"Node4.Relab.Example"}}, true), id: 4},
		{name: "WildcardDNSNeverMatches", ctx: tlsPeerContext(&x509.Certificate{DNSNames: []string{"*.relab.example"}}, true), id: 5, wantErr: `no DNS SAN equal to "node5.relab.example"`},
		{name: "IPSANIgnored", ctx: tlsPeerContext(&x509.Certificate{IPAddresses: []net.IP{net.ParseIP("10.0.0.6")}}, true), id: 6, wantErr: `no DNS SAN equal to "node6.relab.example"`},
		{name: "CommonNameIgnored", ctx: tlsPeerContext(&x509.Certificate{Subject: pkix.Name{CommonName: "node7.relab.example"}}, true), id: 7, wantErr: `no DNS SAN equal to "node7.relab.example"`},
		{name: "UnverifiedCertificate", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, false), id: 2, wantErr: "no verified certificate; the dialer must present its certificate, the server's ClientAuth must verify client certificates, and the dialer must not set InsecureSkipVerify"},
		{name: "NotTLS", ctx: peer.NewContext(context.Background(), &peer.Peer{AuthInfo: otherAuthInfo{}}), id: 2, wantErr: "connection does not use TLS"},
		{name: "NoPeer", ctx: context.Background(), id: 2, wantErr: "no peer information in the stream context"},
		{name: "NoIdentity", ctx: tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node3}}, true), id: 3, wantErr: "no identity configured for node 3"},
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
		node := &tlsNode{"127.0.0.1:9002", "spiffe://relab/node/2"}
		nodes := map[ID]*tlsNode{2: node}
		source := WithNodes(nodes)
		node.identity = "spiffe://relab/node/3"
		delete(nodes, 2)
		verify, err := NewPeerVerifier(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := verify(tlsPeerContext(&x509.Certificate{URIs: []*url.URL{node2}}, true), 2); err != nil {
			t.Errorf("verify(node 2) after the caller changed its map = %v; want nil", err)
		}
	})
}

// TestPeerVerifierTLSConfiguration verifies that construction rejects empty
// identities, identities that no certificate can match, and identities that
// let one SAN match two IDs, naming the lowest such pair, and accepts
// identities that no single SAN matches together.
func TestPeerVerifierTLSConfiguration(t *testing.T) {
	invalid := func(identity, reason string) string {
		return fmt.Sprintf("gorums: node 1 has an invalid TLS identity %q: %s", identity, reason)
	}
	const (
		notASCII = "it must be printable ASCII without spaces"
		address  = "it is a network address, not a certificate name"
		wildcard = "wildcard DNS names never establish identity"
		notName  = "it is neither a URI with a scheme nor a DNS name"
	)
	tests := []struct {
		name       string
		identities map[ID]string
		wantErr    string // empty means no error
	}{
		{
			name:       "DuplicateURI",
			identities: map[ID]string{1: "spiffe://relab/node/1", 2: "spiffe://relab/node/2", 3: "spiffe://relab/node/1"},
			wantErr:    `gorums: nodes 1 and 3 have the same TLS identity "spiffe://relab/node/1"`,
		},
		{
			name:       "DuplicateURICanonicalForm",
			identities: map[ID]string{4: "SPIFFE://relab/node/4", 5: "spiffe://relab/node/4"},
			wantErr:    `gorums: nodes 4 and 5 have the same TLS identity "spiffe://relab/node/4"`,
		},
		{
			name:       "DuplicateDNSIgnoringCase",
			identities: map[ID]string{2: "node2.relab.example", 7: "Node2.Relab.Example"},
			wantErr:    `gorums: nodes 2 and 7 have the same TLS identity "node2.relab.example"`,
		},
		{
			// URI SANs match exactly, so a SAN matches at most one of them.
			name:       "URIsDifferingInCase",
			identities: map[ID]string{1: "spiffe://relab/node/a", 2: "SPIFFE://RELAB/NODE/A"},
		},
		{
			// A URI without an authority, such as a URN, is a valid URI SAN.
			name:       "URN",
			identities: map[ID]string{1: "urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66"},
		},
		{
			name:       "EmptyIdentity",
			identities: map[ID]string{1: ""},
			wantErr:    "gorums: node 1 has an empty TLS identity",
		},
		{
			// U+017F (LATIN SMALL LETTER LONG S) folds to "s", but a
			// certificate cannot carry it.
			name:       "NonASCII",
			identities: map[ID]string{1: "ſpiffe://relab/node/1"},
			wantErr:    invalid("ſpiffe://relab/node/1", notASCII),
		},
		{
			name:       "Space",
			identities: map[ID]string{1: "node 1.relab.example"},
			wantErr:    invalid("node 1.relab.example", notASCII),
		},
		{
			name:       "IPv4Address",
			identities: map[ID]string{1: "10.0.0.1"},
			wantErr:    invalid("10.0.0.1", address),
		},
		{
			name:       "IPv6Address",
			identities: map[ID]string{1: "::1"},
			wantErr:    invalid("::1", address),
		},
		{
			// url.Parse reads "localhost" as a URI scheme.
			name:       "HostPort",
			identities: map[ID]string{1: "localhost:9000"},
			wantErr:    invalid("localhost:9000", address),
		},
		{
			name:       "DNSNamePort",
			identities: map[ID]string{1: "node1.relab.example:9000"},
			wantErr:    invalid("node1.relab.example:9000", address),
		},
		{
			name:       "IPv6Port",
			identities: map[ID]string{1: "[::1]:9000"},
			wantErr:    invalid("[::1]:9000", address),
		},
		{
			name:       "WildcardDNS",
			identities: map[ID]string{1: "*.relab.example"},
			wantErr:    invalid("*.relab.example", wildcard),
		},
		{
			name:       "PathWithoutScheme",
			identities: map[ID]string{1: "relab/node/1"},
			wantErr:    invalid("relab/node/1", notName),
		},
		{
			name:       "EmptyDNSLabel",
			identities: map[ID]string{1: "node1..relab.example"},
			wantErr:    invalid("node1..relab.example", notName),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify, err := NewPeerVerifier(identityNodes(tt.identities))
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr || verify != nil {
					t.Fatalf("NewPeerVerifier = %v, %v; want nil, %q", verify != nil, err, tt.wantErr)
				}
				return
			}
			if err != nil || verify == nil {
				t.Fatalf("NewPeerVerifier = %v, %v; want a verifier and no error", verify != nil, err)
			}
		})
	}
}

// TestPeerVerifierAddressOnly verifies that sources without TLS identities
// retain the unauthenticated behavior.
func TestPeerVerifierAddressOnly(t *testing.T) {
	tests := []struct {
		name  string
		nodes NodeSource
	}{
		{name: "Nil"},
		{name: "Empty", nodes: WithNodes(map[ID]tlsNode{})},
		{name: "List", nodes: WithNodeList([]string{"127.0.0.1:9001"})},
		{name: "Map", nodes: WithNodes(map[ID]mock.NodeAddr{1: "127.0.0.1:9001"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verify, err := NewPeerVerifier(tt.nodes)
			if err != nil || verify != nil {
				t.Errorf("NewPeerVerifier = %v, %v; want nil, nil", verify != nil, err)
			}
		})
	}
}

// TestPeerVerifierWrappedSource verifies that embedding a node source preserves
// its authentication requirement rather than silently disabling verification.
func TestPeerVerifierWrappedSource(t *testing.T) {
	nodes := struct{ NodeSource }{identityNodes(map[ID]string{1: "spiffe://relab/node/1"})}
	verify, err := NewPeerVerifier(nodes)
	if err != nil || verify == nil {
		t.Fatalf("NewPeerVerifier on an embedded source = %v, %v; want a verifier", verify != nil, err)
	}
}
