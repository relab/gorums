package stream

import (
	"context"
)

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by [github.com/relab/gorums/internal/conn.InboundManager].
type PeerAcceptor interface {
	AcceptPeer(ctx context.Context, stream BidiStream) (PeerNode, func(), error)
}

// PeerNode represents a peer from the perspective of stream dispatch.
// Package [github.com/relab/gorums/internal/conn] implements it for an
// identified peer and for a connection whose peer ID is not known.
type PeerNode interface {
	// RouteInbound handles a message received from the peer.
	// Messages with a server-initiated ID (high bit set) are responses to
	// calls this server made; they are delivered to the matching pending call.
	// Messages with a client-initiated ID (low bit) are new requests from
	// the peer; they are passed to the registered handler on the caller's goroutine.
	// release is always called — immediately for server-initiated messages,
	// or by the handler for client-initiated requests.
	RouteInbound(ctx context.Context, msg *Message, release func(), send func(*Message))
	// TrySend delivers a reply to the peer without waiting for send queue
	// space; see [Server.NodeStream]. An implementation without a send queue
	// may block on the underlying transport, which stalls only this peer's
	// connection.
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
// after the previous handler calls release or returns. The receive loop
// delivers replies to this server's calls itself, so the stream is read while
// a handler holds the dispatcher. The loop waits only when the request queue
// is full.
//
// A separate goroutine drains handler replies and delivers each one with
// [PeerNode.TrySend], which never waits for send queue space, so a handler's
// send completes and the handler can release the dispatcher.
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

	requests := newDispatcher(ctx.Done(), 0)

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
		requests.push(ctx, func(release func()) {
			peerNode.RouteInbound(ctx, msg, release, send)
		})
	}
}
