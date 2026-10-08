package conn

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
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
	Logger          *log.Logger
	Backoff         backoff.Config
	SendBufferSize  uint
	Metadata        metadata.MD
	Handler         stream.RequestHandler
	LocalNodeID     uint32          // if non-zero, skip setting handler on this node ID
	StreamDedup     bool            // reuse a lower-ID peer's dialed stream instead of dialing back
	InboundManager  *InboundManager // set when the configuration carries a server (peer or back-channel client); enables eager reconnect and, with StreamDedup, borrowing
	Err             error           // records misuse of a dial option; surfaced by NewConfig
}

// DefaultSendBufferSize is the per-node send queue capacity used when no
// explicit size is configured. Each node allocates the full capacity up front.
const DefaultSendBufferSize = 4096

// NewDialOptions returns a DialOptions initialized with default values.
func NewDialOptions() DialOptions {
	return DialOptions{
		Backoff:        backoff.DefaultConfig,
		SendBufferSize: DefaultSendBufferSize,
	}
}

// WithStreamDedup enables stream deduplication on the outbound manager. It is
// applied by the server when its stream-dedup server option is set.
func WithStreamDedup() DialOption {
	return func(o *DialOptions) {
		o.StreamDedup = true
	}
}
