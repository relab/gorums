package stream

import (
	"context"
)

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by inboundManager in the gorums package.
type PeerAcceptor interface {
	AcceptPeer(ctx context.Context, stream BidiStream) (PeerNode, func(), error)
}

// PeerNode represents a peer from the perspective of stream dispatch.
// It is implemented in the gorums package, by Node for an identified peer and
// by nilPeerNode for a connection whose peer ID is not known.
type PeerNode interface {
	// RouteInbound handles a message received from the peer.
	// Messages with a server-initiated ID (high bit set) are responses to
	// calls this server made; they are delivered to the matching pending call.
	// Messages with a client-initiated ID (low bit) are new requests from
	// the peer; they are dispatched to the registered handler in a new goroutine.
	// release is always called — immediately for server-initiated messages,
	// or by the handler for client-initiated requests.
	RouteInbound(ctx context.Context, msg *Message, release func(), send func(*Message))
	// TrySend delivers a reply to the peer without blocking on a full send
	// queue: see [Server.NodeStream] for why a handler's reply must never be
	// able to block here. An implementation with no queue to fail fast against
	// may still block on the underlying transport; that is safe as long as it
	// only stalls this one peer's connection, not a lock other connections
	// depend on.
	TrySend(req Request)
}

// Server handles NodeStream connections.
type Server struct {
	buffer    uint
	onConnect func(context.Context)
	acceptor  PeerAcceptor
	UnimplementedGorumsServer
}

// NewServer creates a new Server.
func NewServer(buffer uint, onConnect func(context.Context), acceptor PeerAcceptor) *Server {
	return &Server{
		buffer:    buffer,
		onConnect: onConnect,
		acceptor:  acceptor,
	}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
//
// Requests go to a per-stream dispatcher, which starts the next request only
// after the previous handler calls release or returns. This loop delivers
// replies itself, so a handler that has not released does not stop the stream
// from being read. The loop waits only when the request queue is full.
//
// The goroutine below delivers each handler's reply via TrySend, not the
// blocking Enqueue. TrySend must not block: this goroutine is what drains
// finished, and a handler blocked in send would never release the dispatcher.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	finished := make(chan *Message, s.buffer)
	ctx, cancel := context.WithCancel(srv.Context())
	defer cancel()

	peerNode, cleanup, err := s.acceptor.AcceptPeer(ctx, srv)
	if err != nil {
		return err
	}
	defer cleanup()

	if s.onConnect != nil {
		s.onConnect(ctx)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case streamOut := <-finished:
				peerNode.TrySend(Request{Ctx: ctx, Msg: streamOut})
			}
		}
	}()

	requests := newRequestDispatch(defaultRequestDispatchSize)
	go requests.run(ctx)

	for {
		streamIn, err := srv.Recv()
		if err != nil {
			return err
		}

		send := func(msg *Message) {
			select {
			case finished <- msg:
			case <-ctx.Done():
			}
		}
		// A server-initiated ID is a reply to a call this server made.
		// Deliver it before reading further, without the request queue.
		if isServerSequenceNumber(streamIn.GetMessageSeqNo()) {
			peerNode.RouteInbound(ctx, streamIn, func() {}, send)
			continue
		}
		msg := streamIn
		requests.enqueue(ctx, func(release func()) {
			peerNode.RouteInbound(ctx, msg, release, send)
		})
	}
}
