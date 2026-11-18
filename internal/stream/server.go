package stream

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// acceptedKey is the response header that a server sends on each stream it
// accepts, before any message. A dialer counts its stream as up when this
// header or the first frame arrives. A refused stream ends without it.
const acceptedKey = "gorums-accepted"

// streamKey is the stream metadata key that identifies a stream's sender and
// its place among the sender's streams, as "<token>.<n>". The token is random
// per outbound channel, and n counts the channel's streams from 1.
const streamKey = "gorums-stream"

// drainingKey is the stream metadata key that lists, as comma-separated
// numbers, the sender's draining streams whose acceptance the sender has
// received. The server lets them finish before it runs the new stream's
// requests.
const drainingKey = "gorums-draining"

// acceptFrame returns the frame that a server sends on an accepted stream in
// place of the acceptance header, when a stream interceptor has already sent
// the response headers. Message IDs start at 1, so the frame answers no call:
// a dialer counts it as the server's first frame and then drops it.
func acceptFrame() *Message {
	return Message_builder{}.Build()
}

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by [conn.InboundManager].
type PeerAcceptor interface {
	// AcceptPeer returns the channel for an accepted stream and a cleanup
	// function to call when the stream ends, or an error that rejects the
	// stream. ctx is the stream's context.
	AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error)
}

// defaultSenderGrace is how long a server keeps a sender's record after the
// sender's last stream ended.
const defaultSenderGrace = time.Minute

// errStaleStream ends a stream that is not newer than its sender's latest
// stream, before any of its requests run. The sender has already given up such
// a stream and sends on a newer one. codes.Aborted is not a refusal code (see
// isRefusal): the sender fails the stale stream's pending calls with
// [ErrStreamDown], as for a lost stream, and keeps its newer stream without a
// backoff.
var errStaleStream = status.Error(codes.Aborted, "gorums: stream is not newer than the sender's latest stream")

// Server handles NodeStream connections.
//
// A stream from an [OutboundChannel] carries the channel's token and the
// stream's number. For each token, the server runs requests in the order the
// channel sent them, also across the channel's streams: it refuses a stream
// that is not newer than the latest one, and a newer stream supersedes the
// older ones, as described for [Server.NodeStream].
type Server struct {
	onConnect func(context.Context)
	acceptor  PeerAcceptor
	grace     time.Duration // how long to keep a sender's record after its last stream

	mu      sync.Mutex
	senders map[string]*sender // by token
	UnimplementedGorumsServer
}

// NewServer returns a Server that accepts streams with acceptor and calls
// onConnect, if non-nil, for each accepted stream.
func NewServer(onConnect func(context.Context), acceptor PeerAcceptor) *Server {
	return &Server{onConnect: onConnect, acceptor: acceptor, grace: defaultSenderGrace, senders: make(map[string]*sender)}
}

// sender is a server's record of the streams of one outbound channel.
type sender struct {
	// mu orders the acceptance of the sender's streams. It is held while
	// the acceptor accepts a stream, so that a stream is registered before
	// the sender can learn of its acceptance and list it as draining.
	mu      sync.Mutex
	latest  uint64                     // the number of the latest accepted stream
	streams map[uint64]*InboundChannel // channels whose NodeStream has not returned

	refs int    // NodeStream calls for the sender in progress; guarded by Server.mu
	gen  uint64 // counts the calls to acquire; guarded by Server.mu
}

// NodeStream serves one client stream on the channel the acceptor returns for
// it, until receiving from the stream fails or the client closes its side of
// the stream; see [InboundChannel.Serve]. It returns after the handlers of the
// requests that it received have returned.
//
// If the stream carries its sender's token and number, NodeStream refuses it
// with codes.Aborted unless it is newer than the sender's latest stream.
// Otherwise it closes the sender's older channels, except those that the
// stream lists as draining. The stream's first request starts once each older
// channel has run its last request: a closed channel's running request, and
// all requests of a draining channel. Each of these must have returned or
// released. The server keeps a sender's record for a grace period after its
// last stream ended, so that it refuses a late stale stream.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	ctx := srv.Context()
	token, seq, draining, ok := streamIdentity(ctx)
	if !ok {
		ch, cleanup, err := s.acceptor.AcceptPeer(ctx, srv)
		if err != nil {
			return err
		}
		return s.serve(ctx, ch, cleanup)
	}
	snd := s.acquire(token)
	defer s.release(token, snd)

	snd.mu.Lock()
	if seq <= snd.latest {
		snd.mu.Unlock()
		return errStaleStream
	}
	ch, cleanup, err := s.acceptor.AcceptPeer(ctx, srv)
	if err != nil {
		snd.mu.Unlock()
		return err
	}
	snd.latest = seq
	older := maps.Clone(snd.streams)
	snd.streams[seq] = ch
	snd.mu.Unlock()
	defer func() {
		snd.mu.Lock()
		delete(snd.streams, seq)
		snd.mu.Unlock()
	}()

	for n, prev := range older {
		if !slices.Contains(draining, n) {
			_ = prev.Close()
		}
	}
	gate := make(chan struct{})
	go func() {
		for _, prev := range older {
			prev.requests.waitRetired()
		}
		close(gate)
	}()
	ch.requests.hold(gate)
	return s.serve(ctx, ch, cleanup)
}

// serve serves ch until its stream ends, then calls cleanup and waits for the
// stream's handlers.
func (s *Server) serve(ctx context.Context, ch *InboundChannel, cleanup func()) error {
	// gRPC's GracefulStop waits for NodeStream to return, so NodeStream
	// returns only after the stream's handlers have returned. The wait runs
	// after cleanup, which ends the handlers' contexts.
	defer ch.requests.wait(nil)
	defer cleanup()
	if s.onConnect != nil {
		s.onConnect(ctx)
	}
	return ch.Serve()
}

// acquire returns the record of the sender with token, creating it if needed,
// and counts a NodeStream call for it.
func (s *Server) acquire(token string) *sender {
	s.mu.Lock()
	defer s.mu.Unlock()
	snd := s.senders[token]
	if snd == nil {
		snd = &sender{streams: make(map[uint64]*InboundChannel)}
		s.senders[token] = snd
	}
	snd.refs++
	snd.gen++
	return snd
}

// release ends a NodeStream call for the sender with token. After the last
// one, it deletes the sender's record once the grace period has passed
// without a new stream.
func (s *Server) release(token string, snd *sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snd.refs--
	if snd.refs > 0 {
		return
	}
	gen := snd.gen
	time.AfterFunc(s.grace, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if snd.gen == gen && s.senders[token] == snd {
			delete(s.senders, token)
		}
	})
}

// streamIdentity returns the sender token, the stream number, and the
// draining stream numbers in the stream metadata of ctx. It reports false if
// the metadata has no valid stream identity.
func streamIdentity(ctx context.Context) (token string, seq uint64, draining []uint64, ok bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	vals := md.Get(streamKey)
	if len(vals) == 0 {
		return "", 0, nil, false
	}
	token, num, found := strings.Cut(vals[0], ".")
	seq, err := strconv.ParseUint(num, 10, 64)
	if !found || token == "" || err != nil || seq == 0 {
		return "", 0, nil, false
	}
	for _, list := range md.Get(drainingKey) {
		for f := range strings.SplitSeq(list, ",") {
			if n, err := strconv.ParseUint(f, 10, 64); err == nil && n < seq {
				draining = append(draining, n)
			}
		}
	}
	return token, seq, draining, true
}
