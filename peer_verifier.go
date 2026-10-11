package gorums

import "github.com/relab/gorums/internal/conn"

// PeerVerifier reports whether the remote end of a stream is the node with a
// given ID. It returns nil to accept the stream, and an error that explains
// the refusal otherwise. Its context is the stream's context, which carries
// the connection's [google.golang.org/grpc/peer.Peer] and, on a server, the
// stream's incoming metadata. A PeerVerifier must be safe for concurrent use.
// See [WithPeerVerifier].
type PeerVerifier = conn.PeerVerifier
