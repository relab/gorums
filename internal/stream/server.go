package stream

import (
	"context"
	"sync"
)

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by inboundManager in the gorums package.
type PeerAcceptor interface {
	AcceptPeer(ctx context.Context, stream BidiStream) (PeerNode, func(), error)
}

// PeerNode represents a peer from the perspective of stream dispatch.
// It is implemented by Node and nilPeerNode in the gorums package.
type PeerNode interface {
	// RouteInbound handles a message received from the peer.
	// Messages with a server-initiated ID (high bit set) are responses to
	// calls this server made; they are delivered to the matching pending call.
	// Messages with a client-initiated ID (low bit) are new requests from
	// the peer; they are dispatched to the registered handler in a new goroutine.
	// release is always called — immediately for server-initiated messages,
	// or by the handler for client-initiated requests.
	RouteInbound(ctx context.Context, msg *Message, release func(), send func(*Message))
	Enqueue(req Request)
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
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *Message, s.buffer)
	ctx := srv.Context()

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
				peerNode.Enqueue(Request{Ctx: ctx, Msg: streamOut})
			}
		}
	}()

	// Start with a locked mutex
	mut.Lock()
	defer mut.Unlock()

	for {
		streamIn, err := srv.Recv()
		if err != nil {
			return err
		}

		// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
		// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
		// finished, or when it is safe to start processing the next request.
		var once sync.Once
		release := func() { once.Do(mut.Unlock) }
		send := func(msg *Message) {
			select {
			case finished <- msg:
			case <-ctx.Done():
			}
		}

		peerNode.RouteInbound(ctx, streamIn, release, send)

		// Wait until the handler releases the mutex.
		mut.Lock()
	}
}
