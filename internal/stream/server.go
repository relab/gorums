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
	// RouteInbound routes an inbound message on a server-side stream.
	// Returns true if the message was handled (routed to a pending
	// server-initiated call or silently absorbed as stale).
	// Returns false if the message is a new client-initiated request
	// that the caller must dispatch to a handler.
	RouteInbound(msg *Message) bool
	Enqueue(req Request)
}

// Server handles NodeStream connections.
type Server struct {
	buffer    uint
	onConnect func(context.Context)
	acceptor  PeerAcceptor
	handler   RequestHandler
	UnimplementedGorumsServer
}

// NewServer creates a new Server.
func NewServer(buffer uint, onConnect func(context.Context), acceptor PeerAcceptor, handler RequestHandler) *Server {
	return &Server{
		buffer:    buffer,
		onConnect: onConnect,
		acceptor:  acceptor,
		handler:   handler,
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
		// Route responses to pending server-initiated calls; stale calls are discarded.
		// Client-initiated calls will fall through to the handler below.
		if peerNode.RouteInbound(streamIn) {
			continue
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

		go s.handler.HandleRequest(streamIn.AppendToIncomingContext(ctx), streamIn, release, send)

		// Wait until the handler releases the mutex.
		mut.Lock()
	}
}
