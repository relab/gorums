package stream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// handlerAcceptor is a [PeerAcceptor] that accepts each stream on a new
// inbound channel that serves requests with handler.
type handlerAcceptor struct {
	handler RequestHandler
}

func (a handlerAcceptor) AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error) {
	ch := NewInboundChannel(ctx, 0, stream, InboundOptions{SendBufferSize: 10, RecvBufferSize: testRecvBufferSize, Handler: a.handler})
	return ch, func() { _ = ch.Close() }, nil
}

// gatedAcceptor is a [PeerAcceptor] that counts its calls and, if entered is
// non-nil, reports each call on entered and waits for release before it
// accepts the stream.
type gatedAcceptor struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (a *gatedAcceptor) AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error) {
	a.calls.Add(1)
	if a.entered != nil {
		a.entered <- struct{}{}
		<-a.release
	}
	return handlerAcceptor{}.AcceptPeer(ctx, stream)
}

// peerDraining reports whether a draining frame arrived on one of the
// channel's open sessions.
func (c *OutboundChannel) peerDraining() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for s := range c.sessions {
		if s.peerDraining.Load() {
			return true
		}
	}
	return false
}

// drainStream sends the draining frame on stream, waits for the final frame,
// and sends the drained frame, as a draining server does.
func drainStream(stream Gorums_NodeStreamServer) error {
	if err := stream.Send(drainFrame(drainingMethod)); err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if msg.GetMethod() == drainFinalMethod {
			break
		}
	}
	return stream.Send(drainFrame(drainedMethod))
}

// TestDrainSentCallFailsWithStreamDown verifies that a two-way call that was
// sent before the drain, and whose reply never arrives, does not fail with
// ErrServerDraining: the server may have run it, so it is not safe to retry.
func TestDrainSentCallFailsWithStreamDown(t *testing.T) {
	tc := setupChannel(t, func(stream Gorums_NodeStreamServer) error {
		acceptStream(stream)
		if _, err := stream.Recv(); err != nil { // the call; no reply
			return err
		}
		if err := drainStream(stream); err != nil {
			return err
		}
		_, err := stream.Recv() // until the client ends the stream
		return err
	})

	resp := sendRequest(t, tc.OutboundChannel, NodeRequest{}, 1)
	if errors.Is(resp.Err, ErrServerDraining) {
		t.Fatalf("sent call failed with %v; want an error that does not claim the call did not run", resp.Err)
	}
	if !errors.Is(resp.Err, ErrStreamDown) {
		t.Errorf("sent call error = %v, want %v", resp.Err, ErrStreamDown)
	}
}

// TestDrainRefusalPersistsUntilAccepted verifies that after a server drained
// the channel, sends fail with ErrServerDraining while the server refuses new
// streams, also once the first stream has ended, and that sends resume when a
// replacement server accepts a stream.
func TestDrainRefusalPersistsUntilAccepted(t *testing.T) {
	var refusing atomic.Bool
	streamEnded := make(chan struct{})
	received := make(chan *Message, 10)
	var streams atomic.Int32
	tc := setupChannel(t, func(stream Gorums_NodeStreamServer) error {
		if refusing.Load() {
			return ErrServerDraining
		}
		acceptStream(stream)
		if streams.Add(1) == 1 {
			defer close(streamEnded)
			if err := drainStream(stream); err != nil {
				return err
			}
			refusing.Store(true)
			_, err := stream.Recv() // until the client ends the stream
			return err
		}
		for {
			msg, err := stream.Recv()
			if err != nil {
				return err
			}
			received <- msg
		}
	})

	select {
	case <-streamEnded:
	case <-time.After(defaultTestTimeout):
		t.Fatal("the client did not end the drained stream")
	}
	resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, 1)
	if !errors.Is(resp.Err, ErrServerDraining) {
		t.Fatalf("one-way send to a drained server: err = %v, want %v", resp.Err, ErrServerDraining)
	}

	refusing.Store(false) // a replacement server
	if !waitUntil(defaultTestTimeout, tc.StreamUp) {
		t.Fatal("the channel did not reconnect to the replacement server")
	}
	if resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, 2); resp.Err != nil {
		t.Fatalf("one-way send to the replacement server: %v", resp.Err)
	}
	select {
	case msg := <-received:
		if msg.GetMessageSeqNo() != 2 {
			t.Errorf("replacement server received message %d, want 2", msg.GetMessageSeqNo())
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("the replacement server did not receive the request")
	}
}

// TestDrainFinalOnStoppedSession verifies that a session whose send loop has
// stopped, as after a GOAWAY, sends the final frame on its own stream when
// the draining frame arrives, instead of queuing it for another session.
func TestDrainFinalOnStoppedSession(t *testing.T) {
	e := newEndpoint(t.Context(), 1, 10, testRecvBufferSize, nil, nil)
	st := mock.NewRecordingBidiStream[*Message]()
	t.Cleanup(st.Close)
	ctx, cancel := context.WithCancel(e.ctx)
	t.Cleanup(cancel)
	s := newSession(ctx, cancel, &e, st, true, true)

	// A pending call keeps the session open after its send loop stops.
	call := NodeRequest{Ctx: t.Context(), Msg: drainFrame(mock.TestMethod), ResponseChan: make(chan response, 1)}
	call.Msg.SetMessageSeqNo(1)
	s.pending.add(1, call)
	s.startDrain()
	if req := s.sendLoop(nil); req != nil {
		t.Fatalf("sendLoop returned request %d, want none", req.Msg.GetMessageSeqNo())
	}

	go func() { _ = s.receive() }()
	if err := st.Deliver(drainFrame(drainingMethod)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	select {
	case msg := <-st.Sent():
		if msg.GetMethod() != drainFinalMethod {
			t.Errorf("sent %q, want %q", msg.GetMethod(), drainFinalMethod)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("the stopped session did not send the final frame")
	}
	if n := len(e.queue.ch); n != 0 {
		t.Errorf("send queue holds %d frames, want 0", n)
	}
	if s.ended() {
		t.Error("session ended before the drained frame arrived")
	}
}

// TestDrainNoticeIsCutoff verifies that a request queued but not yet sent
// when the draining frame arrives fails with ErrServerDraining, and that the
// final frame follows the last request sent.
func TestDrainNoticeIsCutoff(t *testing.T) {
	e := newEndpoint(t.Context(), 1, 10, testRecvBufferSize, nil, nil)
	st := mock.NewGatedBidiStream[*Message]()
	t.Cleanup(st.Close)
	ctx, cancel := context.WithCancel(e.ctx)
	t.Cleanup(cancel)
	s := newSession(ctx, cancel, &e, st, true, true)
	go s.sendLoop(nil)
	go func() { _ = s.receive() }()

	enqueue := func(id uint64) chan response {
		respCh := make(chan response, 1)
		msg := drainFrame(mock.TestMethod)
		msg.SetMessageSeqNo(id)
		e.Enqueue(NodeRequest{Ctx: t.Context(), Msg: msg, Oneway: true, ResponseChan: respCh})
		return respCh
	}
	sent := enqueue(1)
	select {
	case <-st.Entered(): // request 1 is in Send
	case <-time.After(defaultTestTimeout):
		t.Fatal("request 1 did not reach Send")
	}
	queued := enqueue(2)
	if err := st.Deliver(drainFrame(drainingMethod)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if !waitUntil(defaultTestTimeout, s.peerDraining.Load) {
		t.Fatal("the draining frame was not handled")
	}
	st.Release()

	if resp := <-sent; resp.Err != nil {
		t.Errorf("request sent before the notice: %v", resp.Err)
	}
	if resp := <-queued; !errors.Is(resp.Err, ErrServerDraining) {
		t.Errorf("request queued before the notice: err = %v, want %v", resp.Err, ErrServerDraining)
	}
	var got []string
	for len(got) < 2 {
		select {
		case msg := <-st.Sent():
			got = append(got, msg.GetMethod())
		case <-time.After(defaultTestTimeout):
			t.Fatalf("sent %v, want [%s %s]", got, mock.TestMethod, drainFinalMethod)
		}
	}
	if got[0] != mock.TestMethod || got[1] != drainFinalMethod {
		t.Errorf("sent %v, want [%s %s]", got, mock.TestMethod, drainFinalMethod)
	}
}

// TestDrainWaitsForWorkAfterDisconnect verifies that Drain waits for a
// running and a queued handler after the dialer closes its stream, and that
// the queued handler still runs.
func TestDrainWaitsForWorkAfterDisconnect(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	var finished atomic.Int32
	srv := NewServer(nil, handlerAcceptor{handler: requestHandlerFunc(func(context.Context, *Message, func(), func(*Message)) {
		started <- struct{}{}
		<-gate
		finished.Add(1)
	})})
	tc := setupChannel(t, srv.NodeStream)

	for id := range uint64(2) {
		if resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, id+1); resp.Err != nil {
			t.Fatalf("one-way send %d: %v", id+1, resp.Err)
		}
	}
	<-started // the first handler runs; the second is queued

	ctx, cancel := context.WithTimeout(t.Context(), defaultTestTimeout)
	defer cancel()
	drained := make(chan bool, 1)
	go func() { drained <- srv.Drain(ctx) }()
	if !waitUntil(defaultTestTimeout, tc.peerDraining) {
		t.Fatal("the client did not learn of the drain")
	}
	if err := tc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-drained:
		t.Fatal("Drain returned while accepted handlers were blocked")
	case <-time.After(200 * time.Millisecond):
	}
	close(gate)
	select {
	case ok := <-drained:
		if !ok {
			t.Error("Drain = false, want true")
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("Drain did not return after the handlers finished")
	}
	if got := finished.Load(); got != 2 {
		t.Errorf("finished handlers = %d, want 2", got)
	}
}

// TestDrainAdmission verifies that a draining server refuses a new stream
// before its acceptor sees it, and that a stream admitted before the drain
// began is drained too.
func TestDrainAdmission(t *testing.T) {
	t.Run("RefusedBeforeAccept", func(t *testing.T) {
		acceptor := &gatedAcceptor{}
		srv := NewServer(nil, acceptor)
		if !srv.Drain(t.Context()) {
			t.Fatal("Drain of an idle server = false, want true")
		}
		tc := setupChannel(t, srv.NodeStream)
		if !waitUntil(defaultTestTimeout, func() bool { return isServerDraining(tc.LastErr()) }) {
			t.Fatalf("LastErr = %v, want %v", tc.LastErr(), ErrServerDraining)
		}
		if n := acceptor.calls.Load(); n != 0 {
			t.Errorf("acceptor called %d times, want 0", n)
		}
	})
	t.Run("AdmittedBeforeDrain", func(t *testing.T) {
		acceptor := &gatedAcceptor{entered: make(chan struct{}, 1), release: make(chan struct{})}
		srv := NewServer(nil, acceptor)
		tc := setupChannel(t, srv.NodeStream)
		select {
		case <-acceptor.entered:
		case <-time.After(defaultTestTimeout):
			t.Fatal("the stream did not reach the acceptor")
		}

		ctx, cancel := context.WithTimeout(t.Context(), defaultTestTimeout)
		defer cancel()
		drained := make(chan bool, 1)
		go func() { drained <- srv.Drain(ctx) }()
		select {
		case <-drained:
			t.Fatal("Drain returned while a stream was being accepted")
		case <-time.After(100 * time.Millisecond):
		}
		close(acceptor.release)
		// Drain returns true only once the admitted stream sent its drained
		// frame or ended.
		if ok := <-drained; !ok {
			t.Fatal("Drain = false, want true")
		}
		if tc.StreamUp() && !waitUntil(defaultTestTimeout, func() bool { return !tc.StreamUp() }) {
			t.Error("the client kept the drained stream up")
		}
	})
}
