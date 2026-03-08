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
// It is implemented by Node in the gorums package.
type PeerNode interface {
	RouteResponse(msg *Message) bool
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
				if peerNode != nil {
					// Route through Channel.sender() — sole writer on inbound stream.
					peerNode.Enqueue(Request{Ctx: ctx, Msg: streamOut})
				} else if err := srv.Send(streamOut); err != nil {
					return
				}
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
		// Route response to a pending server-initiated call.
		// This must be checked before handler dispatch: a message with a
		// matching msgID in the router's pending map is a response, not
		// a new request, even if it has a method field set.
		if peerNode != nil && peerNode.RouteResponse(streamIn) {
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
