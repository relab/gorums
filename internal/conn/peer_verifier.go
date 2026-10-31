package conn

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// PeerVerifier reports whether the remote end of a connection is the node
// with ID id. ctx carries the remote end's [peer.Peer]. On a server, ctx is
// the stream's context, which also carries the stream's incoming metadata. On
// a dialer, the peer holds the AuthInfo of the TLS handshake. A PeerVerifier
// returns nil to accept the remote end, and an error that explains the
// refusal otherwise. It must be safe for concurrent use and should return
// promptly.
type PeerVerifier func(ctx context.Context, id ID) error

// verificationError is the error for a remote end that failed a
// [PeerVerifier] check as node id. Its gRPC status code is
// codes.Unauthenticated.
type verificationError struct {
	id  ID
	err error
}

func (e verificationError) Error() string {
	return fmt.Sprintf("gorums: node %d failed verification: %v", e.id, e.err)
}

// GRPCStatus returns e as a codes.Unauthenticated status. gRPC uses it for a
// stream that a server refuses with e, and for a call on a connection whose
// handshake failed with e.
func (e verificationError) GRPCStatus() *status.Status {
	return status.New(codes.Unauthenticated, e.Error())
}

// verifiedCredentials are transport credentials that check the server at the
// other end of each connection with verify, right after the TLS handshake and
// before the connection carries any stream. If the check fails, the
// connection is closed, and the handshake fails with a [verificationError].
type verifiedCredentials struct {
	credentials.TransportCredentials
	id     ID // the node that each connection dials
	verify PeerVerifier
}

// ClientHandshake runs the wrapped handshake and checks its result.
func (c verifiedCredentials) ClientHandshake(ctx context.Context, authority string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, authInfo, err := c.TransportCredentials.ClientHandshake(ctx, authority, rawConn)
	if err != nil {
		return nil, nil, err
	}
	if err := c.verify(peer.NewContext(ctx, &peer.Peer{AuthInfo: authInfo}), c.id); err != nil {
		_ = conn.Close()
		return nil, nil, verificationError{id: c.id, err: err}
	}
	return conn, authInfo, nil
}

// Clone returns a copy of c that wraps a clone of c's credentials.
func (c verifiedCredentials) Clone() credentials.TransportCredentials {
	c.TransportCredentials = c.TransportCredentials.Clone()
	return c
}

// peerIdentity is the name that a node's certificate must carry: a URI SAN
// if uri is set, and a DNS SAN otherwise.
type peerIdentity struct {
	name string
	uri  bool
}

// same reports whether p and q name the same SAN, so that one certificate
// matches both.
func (p peerIdentity) same(q peerIdentity) bool {
	if p.uri != q.uri {
		return false
	}
	if p.uri {
		return p.name == q.name
	}
	return strings.EqualFold(p.name, q.name)
}

// kind returns the type of SAN that the identity names.
func (p peerIdentity) kind() string {
	if p.uri {
		return "URI"
	}
	return "DNS"
}

// parseIdentity returns the SAN that a node's certificate must carry for
// identity s. An identity with a URI scheme names a URI SAN, and any other
// identity must be a DNS name. It rejects an identity that no certificate SAN
// can match: text that is not printable ASCII, a network address, or a
// wildcard DNS name.
func parseIdentity(s string) (peerIdentity, error) {
	for i := range len(s) {
		if s[i] <= ' ' || s[i] > '~' {
			return peerIdentity{}, errors.New("it must be printable ASCII without spaces")
		}
	}
	if isNetworkAddress(s) {
		return peerIdentity{}, errors.New("it is a network address, not a certificate name")
	}
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		return peerIdentity{name: u.String(), uri: true}, nil
	}
	if strings.Contains(s, "*") {
		return peerIdentity{}, errors.New("wildcard DNS names never establish identity")
	}
	if !isDNSName(s) {
		return peerIdentity{}, errors.New("it is neither a URI with a scheme nor a DNS name")
	}
	return peerIdentity{name: s}, nil
}

// isNetworkAddress reports whether s is an IP address, or a host with a
// numeric port. Such a string names where a node listens, not who it is.
func isNetworkAddress(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	_, port, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	_, err = strconv.ParseUint(port, 10, 16)
	return err == nil
}

// isDNSName reports whether s consists of nonempty, dot-separated labels of
// ASCII letters, digits, hyphens, and underscores.
func isDNSName(s string) bool {
	for label := range strings.SplitSeq(s, ".") {
		if label == "" {
			return false
		}
		for i := range len(label) {
			switch c := label[i]; {
			case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '-', c == '_':
			default:
				return false
			}
		}
	}
	return true
}

// NewPeerVerifier returns a TLS verifier for nodes that declare [TLSIdentity],
// or nil for an address-only source. If any node declares an identity, every
// node must declare a nonempty identity that a certificate SAN can match (see
// [parseIdentity]), and no two IDs may name the same SAN. The verifier accepts
// a remote end only if its verified certificate has the expected SAN.
func NewPeerVerifier(nodes NodeSource) (PeerVerifier, error) {
	if nodes == nil {
		return nil, nil
	}
	nm := nodes.nodeMap()
	if !nm.hasIdentity() {
		return nil, nil
	}
	want := make(map[ID]peerIdentity, len(nm))
	ids := slices.Sorted(maps.Keys(nm))
	for _, id := range ids {
		node := nm[id]
		if !node.hasIdentity {
			return nil, fmt.Errorf("gorums: node %d does not implement TLSIdentity", id)
		}
		if node.identity == "" {
			return nil, fmt.Errorf("gorums: node %d has an empty TLS identity", id)
		}
		identity, err := parseIdentity(node.identity)
		if err != nil {
			return nil, fmt.Errorf("gorums: node %d has an invalid TLS identity %q: %w", id, node.identity, err)
		}
		want[id] = identity
	}
	// IDs are compared in increasing order, so the error names the same pair
	// on every run.
	for i, id := range ids {
		for _, lower := range ids[:i] {
			if want[lower].same(want[id]) {
				return nil, fmt.Errorf("gorums: nodes %d and %d have the same TLS identity %q", lower, id, want[lower].name)
			}
		}
	}
	return func(ctx context.Context, id ID) error {
		p, ok := peer.FromContext(ctx)
		if !ok {
			return errors.New("no peer information in the stream context")
		}
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return errors.New("connection does not use TLS")
		}
		chains := tlsInfo.State.VerifiedChains
		if len(chains) == 0 || len(chains[0]) == 0 {
			return errors.New("no verified certificate; the dialer must present its certificate, the server's ClientAuth must verify client certificates, and the dialer must not set InsecureSkipVerify")
		}
		identity, ok := want[id]
		if !ok {
			return fmt.Errorf("no identity configured for node %d", id)
		}
		// Every verified chain starts with the certificate the remote end sent.
		cert := chains[0][0]
		if identity.uri {
			for _, u := range cert.URIs {
				if u.String() == identity.name {
					return nil
				}
			}
		} else {
			// A DNS identity holds no "*", so a wildcard SAN never matches it.
			for _, name := range cert.DNSNames {
				if strings.EqualFold(name, identity.name) {
					return nil
				}
			}
		}
		return fmt.Errorf("certificate has no %s SAN equal to %q", identity.kind(), identity.name)
	}, nil
}
