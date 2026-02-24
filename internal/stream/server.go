package stream

import (
	"context"
	"sync"
)

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by InboundManager in the gorums package.
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
	handlers  map[string]Handler
	buffer    uint
	acceptor  PeerAcceptor
	onConnect func(context.Context)
	UnimplementedGorumsServer
}

// NewServer creates a new Server.
func NewServer(buffer uint, acceptor PeerAcceptor, onConnect func(context.Context)) *Server {
	return &Server{
		handlers:  make(map[string]Handler),
		buffer:    buffer,
		acceptor:  acceptor,
		onConnect: onConnect,
	}
}

// RegisterHandler registers a request handler for the specified method name.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.handlers[method] = handler
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *Message, s.buffer)
	ctx := srv.Context()

	var peerNode PeerNode
	if s.acceptor != nil {
		node, cleanup, err := s.acceptor.AcceptPeer(ctx, srv)
		if err != nil {
			return err
		}
		if node != nil {
			peerNode = node
			defer cleanup()
		}
	}

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
		if handler, ok := s.handlers[streamIn.GetMethod()]; ok {
			// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
			// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
			// finished, or when it is safe to start processing the next request.
			//
			// This func() is the default interceptor; it is the first and last handler in the chain.
			// It is responsible for releasing the mutex when the handler chain is done.
			go func() {
				srvCtx := NewServerCtx(streamIn.AppendToIncomingContext(ctx), &mut, finished)
				defer srvCtx.Release()

				msg, err := UnmarshalRequest(streamIn)
				in := &Envelope{Msg: msg, Message: streamIn}
				if err != nil {
					_ = srvCtx.SendMessage(MessageWithError(in, nil, err))
					return
				}

				out, err := handler(srvCtx, in)
				// If there is no response and no error, we do not send anything back to the client.
				// This corresponds to a unidirectional message from client to server, where clients
				// are not expected to receive a response.
				if out == nil && err == nil {
					return
				}
				_ = srvCtx.SendMessage(MessageWithError(in, out, err))
				// We ignore the error from SendMessage here; it means that the stream is closed.
				// The for-loop above will exit on the next Recv call.
			}()
			// Wait until the handler releases the mutex.
			mut.Lock()
		}
	}
}
