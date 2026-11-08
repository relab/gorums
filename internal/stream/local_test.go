package stream

import (
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestLocalChannelReentrantCall verifies that a local handler which calls its
// own node before releasing waits only as long as the nested call's deadline
// allows, and that the nested call runs once the handler returns. The nested
// handler must receive the nested call's context, which has expired by then.
func TestLocalChannelReentrantCall(t *testing.T) {
	var c *LocalChannel
	nestedResponseChan := make(chan response, 1)
	nestedCtxErr := make(chan error, 1)
	outerDone := make(chan error, 1)
	handler := requestHandlerFunc(func(ctx context.Context, msg *Message, _ func(), send func(*Message)) {
		if msg.GetMessageSeqNo() != 1 {
			nestedCtxErr <- ctx.Err()
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo()}.Build())
			return
		}
		nestedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		c.Enqueue(NodeRequest{
			Ctx:          nestedCtx,
			Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
			ResponseChan: nestedResponseChan,
		})
		select {
		case <-nestedResponseChan:
			outerDone <- errors.New("nested call ran before the outer handler released")
		case <-nestedCtx.Done():
			outerDone <- nil
		}
	})
	c = NewLocalChannel(1, handler, testRecvBufferSize, nil)

	c.Enqueue(NodeRequest{
		Ctx:          t.Context(),
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: make(chan response, 1),
	})
	select {
	case err := <-outerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("outer handler deadlocked on its nested call")
	}
	select {
	case err := <-nestedCtxErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("nested handler context error = %v, want %v", err, context.DeadlineExceeded)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("nested call did not run after the outer handler returned")
	}
	select {
	case <-nestedResponseChan:
	case <-time.After(defaultTestTimeout):
		t.Fatal("nested call did not respond after the outer handler returned")
	}
}

// TestLocalChannelUnmatchedErrors verifies that an error reply to a one-way
// request is counted and logged, while an error reply to a two-way request is
// delivered to its caller and not counted.
func TestLocalChannelUnmatchedErrors(t *testing.T) {
	handler := requestHandlerFunc(func(_ context.Context, msg *Message, _ func(), send func(*Message)) {
		send(Message_builder{
			MessageSeqNo: msg.GetMessageSeqNo(),
			Method:       msg.GetMethod(),
			Status:       status.New(codes.Unimplemented, "no handler registered").Proto(),
		}.Build())
	})
	var logBuf lockedBuffer
	c := NewLocalChannel(1, handler, testRecvBufferSize, log.New(&logBuf, "", 0))

	responseChan := make(chan response, 1)
	c.Enqueue(NodeRequest{
		Ctx:          t.Context(),
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: responseChan,
	})
	select {
	case resp := <-responseChan:
		if status.Code(resp.Err) != codes.Unimplemented {
			t.Errorf("two-way error = %v, want codes.Unimplemented", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("two-way request got no response")
	}
	if got := c.UnmatchedErrors(); got != 0 {
		t.Errorf("UnmatchedErrors() = %d after a two-way error, want 0", got)
	}

	c.Enqueue(NodeRequest{
		Ctx:    t.Context(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
	})
	if !waitUntil(defaultTestTimeout, func() bool { return c.UnmatchedErrors() == 1 }) {
		t.Fatalf("UnmatchedErrors() = %d after a one-way error, want 1", c.UnmatchedErrors())
	}
	logged := logBuf.String()
	for _, want := range []string{"node 1", mock.TestMethod, "message 2", "Unimplemented"} {
		if !strings.Contains(logged, want) {
			t.Errorf("logged %q, want it to contain %q", logged, want)
		}
	}
}

// TestLocalChannelUnmatchedErrorsEndedCall verifies that an error reply to a
// two-way request whose context ended while the handler ran is counted and
// logged.
func TestLocalChannelUnmatchedErrorsEndedCall(t *testing.T) {
	proceed := make(chan struct{})
	handler := requestHandlerFunc(func(_ context.Context, msg *Message, _ func(), send func(*Message)) {
		<-proceed
		send(Message_builder{
			MessageSeqNo: msg.GetMessageSeqNo(),
			Method:       msg.GetMethod(),
			Status:       status.New(codes.Unimplemented, "no handler registered").Proto(),
		}.Build())
	})
	var logBuf lockedBuffer
	c := NewLocalChannel(1, handler, testRecvBufferSize, log.New(&logBuf, "", 0))

	ctx, cancel := context.WithCancel(t.Context())
	c.Enqueue(NodeRequest{
		Ctx:          ctx,
		Msg:          Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build(),
		ResponseChan: make(chan response, 1),
	})
	cancel()
	close(proceed)
	if !waitUntil(defaultTestTimeout, func() bool { return c.UnmatchedErrors() == 1 }) {
		t.Fatalf("UnmatchedErrors() = %d after an error reply to an ended call, want 1", c.UnmatchedErrors())
	}
	if logged := logBuf.String(); !strings.Contains(logged, "message 3") {
		t.Errorf("logged %q, want it to contain %q", logged, "message 3")
	}
}
