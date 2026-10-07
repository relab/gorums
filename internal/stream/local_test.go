package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// TestLocalChannelReentrantCall verifies that a local handler which calls its
// own node before releasing waits only as long as its context allows, and that
// the nested call runs once the handler returns.
func TestLocalChannelReentrantCall(t *testing.T) {
	var c *LocalChannel
	nestedResponseChan := make(chan response, 1)
	outerDone := make(chan error, 1)
	handler := requestHandlerFunc(func(ctx context.Context, msg *Message, _ func(), send func(*Message)) {
		if msg.GetMessageSeqNo() != 1 {
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo()}.Build())
			return
		}
		nestedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		c.Enqueue(Request{
			Ctx:          context.Background(),
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
	c = NewLocalChannel(1, handler)

	c.Enqueue(Request{
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
	case <-nestedResponseChan:
	case <-time.After(defaultTestTimeout):
		t.Fatal("nested call did not run after the outer handler returned")
	}
}
