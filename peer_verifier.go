package gorums

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// PeerVerifier reports whether the remote end of a stream is the node with a
// given ID. It returns nil to accept the stream, and an error that explains
// the refusal otherwise. Its context is the stream's context, which carries
// the connection's [google.golang.org/grpc/peer.Peer] and, on a server, the
// stream's incoming metadata. A PeerVerifier must be safe for concurrent use.
// It should return promptly, and return when its context ends: on a server
// that dials a peer, it runs on the goroutine that sends to that peer, so a
// verifier that blocks delays the sends to that peer and [Server.Stop]. See
// [WithPeerVerifier].
type PeerVerifier = conn.PeerVerifier

// peerIdentity is the name that a node's certificate must carry: a URI SAN
// if uri is set, and a DNS SAN otherwise.
type peerIdentity struct {
	name string
	uri  bool
}

// kind returns the type of SAN that the identity names.
func (p peerIdentity) kind() string {
	if p.uri {
		return "URI"
	}
	return "DNS"
}

// TLSPeerVerifier returns a [PeerVerifier] that accepts a stream for node ID
// id only if the TLS handshake verified the remote end's certificate against
// a trusted CA, and that certificate has a SAN that matches identities[id].
// An identity with a URI scheme, such as spiffe://relab/node/2, matches only
// a URI SAN with the same canonical form. Any other identity matches only a
// DNS SAN, without regard to case. A wildcard DNS SAN never matches, IP SANs
// and the Common Name are never used, and a node ID with an empty or missing
// identity is refused.
//
// The server's TLS configuration must verify client certificates, with
// ClientAuth set to tls.VerifyClientCertIfGiven or
// tls.RequireAndVerifyClientCert, and each replica's certificate must be
// valid for both client and server authentication. TLSPeerVerifier copies
// identities.
func TLSPeerVerifier(identities map[ID]string) PeerVerifier {
	want := make(map[ID]peerIdentity, len(identities))
	for id, name := range identities {
		if name == "" {
			continue // An empty identity is treated as missing.
		}
		if u, err := url.Parse(name); err == nil && u.Scheme != "" {
			want[id] = peerIdentity{name: u.String(), uri: true}
			continue
		}
		want[id] = peerIdentity{name: name}
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
			return errors.New("no verified certificate")
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
			for _, name := range cert.DNSNames {
				if !strings.HasPrefix(name, "*") && strings.EqualFold(name, identity.name) {
					return nil
				}
			}
		}
		return fmt.Errorf("certificate has no %s SAN equal to %q", identity.kind(), identity.name)
	}
}
