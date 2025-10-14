package gorums

import (
	"testing"
	"time"

	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockSrv struct{}

func (mockSrv) Test(_ ServerCtx, _ *mock.Request) (*mock.Response, error) {
	return nil, nil
}

func newNode(t *testing.T, srvAddr string) *RawNode {
	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	t.Cleanup(mgr.Close)
	node, err := NewRawNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Error(err)
	}
	return node
}

var handlerName = "mock.Server.Test"

func dummySrv() *Server {
	mockSrv := &mockSrv{}
	srv := NewServer()
	srv.RegisterHandler(handlerName, func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(*mock.Request)
		defer ctx.Release()
		resp, err := mockSrv.Test(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, resp, err))
	})
	return srv
}

func TestChannelCreation(t *testing.T) {
	node := newNode(t, "127.0.0.1:5000")

	// Send a single message through the channel
	sendRequest(t, node, t.Context(), 1, callOptions{}, 3*time.Second)
}

func TestChannelReconnection(t *testing.T) {
	srvAddr := "127.0.0.1:5000"
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())

	node := newNode(t, srvAddr)

	// send message when server is down
	resp := sendRequest(t, node, t.Context(), 1, callOptions{}, 3*time.Second)
	if resp.err == nil {
		t.Error("response err: got <nil>, want error")
	}

	startServer()

	// send message when server is up but not yet connected,
	// with retries to accommodate gRPC connection establishment
	var successfulSend bool
	for range 10 {
		resp = sendRequest(t, node, t.Context(), 2, getCallOptions(E_Multicast, nil), 3*time.Second)
		if resp.err == nil {
			successfulSend = true
			break
		}
		// server is up but gRPC connection not yet established, retry after delay
		time.Sleep(500 * time.Millisecond)
	}
	if !successfulSend {
		t.Error("failed to send message after server came back up")
	}

	stopServer()

	// send third message when server has previously been up, but is now down
	resp = sendRequest(t, node, t.Context(), 3, callOptions{}, 3*time.Second)
	if resp.err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

func TestEnqueueToClosedChannel(t *testing.T) {
	node := newNode(t, "127.0.0.1:5000")

	// Close the manager (which closes the channel)
	node.mgr.Close()

	// Allow some time for the context to be cancelled
	time.Sleep(50 * time.Millisecond)

	// Try to send a message after the channel is closed
	// Should receive an error response
	resp := sendRequest(t, node, t.Context(), 1, callOptions{}, 3*time.Second)
	if resp.err == nil {
		t.Error("expected error when enqueueing to closed channel, got nil")
	}
	if resp.err.Error() != "channel closed" {
		t.Errorf("expected 'channel closed' error, got: %v", resp.err)
	}
}
