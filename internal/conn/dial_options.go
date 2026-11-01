package conn

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/relab/gorums/internal/stream"
)

// DialOption sets an option on a configuration's [DialOptions].
type DialOption func(*DialOptions)

// DialOptions holds the accumulated dial configuration for a [Config]'s
// connections. Its fields are populated by the option constructors in package
// gorums and consumed by the outbound manager when it builds nodes.
type DialOptions struct {
	GRPCDialOptions []grpc.DialOption
	Credentials     credentials.TransportCredentials // if non-nil, replaces the credentials in GRPCDialOptions; nodes with TLS identities require it
	Logger          *log.Logger
	Backoff         backoff.Config
	SendBufferSize  uint
	RecvBufferSize  uint
	Metadata        metadata.MD
	Handler         stream.RequestHandler
	LocalNodeID     ID              // if non-zero, skip setting handler on this node ID
	StreamDedup     bool            // reuse a lower-ID peer's dialed stream instead of dialing back
	InboundManager  *InboundManager // set when the configuration carries a server (peer or back-channel client); enables eager reconnect and, with StreamDedup, borrowing
	Err             error           // records misuse of a dial option; surfaced by NewConfig
}

// inProcess reports whether calls to node id run in-process: id is this
// server's own node, and a handler serves the calls.
func (o *DialOptions) inProcess(id ID) bool {
	return id == o.LocalNodeID && o.Handler != nil
}

// borrowsStream reports whether node id sends its calls on the stream that the
// peer dials to this server, instead of dialing the peer. Under stream
// deduplication, the peer with the lower ID dials.
func (o *DialOptions) borrowsStream(id ID) bool {
	return o.StreamDedup && o.InboundManager != nil && id < o.LocalNodeID
}

// DefaultSendBufferSize is the per-node send queue capacity used when no
// explicit size is configured. Each node allocates the full capacity up front.
const DefaultSendBufferSize = 4096

// DefaultRecvBufferSize is the number of received requests per channel that can
// wait for the running handler when no explicit size is configured.
const DefaultRecvBufferSize = 4096

// NewDialOptions returns a DialOptions initialized with default values.
func NewDialOptions() DialOptions {
	return DialOptions{
		Backoff:        backoff.DefaultConfig,
		SendBufferSize: DefaultSendBufferSize,
		RecvBufferSize: DefaultRecvBufferSize,
	}
}

// WithStreamDedup enables stream deduplication on the outbound manager. It is
// applied by the server when its stream-dedup server option is set.
func WithStreamDedup() DialOption {
	return func(o *DialOptions) {
		o.StreamDedup = true
	}
}
