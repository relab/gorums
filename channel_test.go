package gorums

import (
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockSrv struct{}

func (mockSrv) Test(_ ServerCtx, _ *mock.Request) (*mock.Response, error) {
	return nil, nil
}

func dummyMgr() *RawManager {
	return NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
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

// sendTestMessage sends a test message via the node's channel and returns a response channel.
func sendTestMessage(t *testing.T, node *RawNode, msgID uint64, opts callOptions) <-chan response {
	t.Helper()
	replyChan := make(chan response, 1)
	go func() {
		ctx := t.Context()
		md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
		req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}, opts: opts}
		node.channel.enqueue(req, replyChan, false)
	}()
	return replyChan
}

// expectResponse waits for a response from replyChan and validates it using the provided check function.
// The check function returns true if the response is acceptable, false otherwise.
// Returns the check result (can be ignored when not needed for retry logic).
func expectResponse(t *testing.T, replyChan <-chan response, check func(t *testing.T, resp response) bool) bool {
	t.Helper()
	select {
	case resp := <-replyChan:
		return check(t, resp)
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: unable to dequeue messages from the node")
	}
	return false // unreachable, but keeps compiler happy
}

func TestChannelCreation(t *testing.T) {
	node, err := NewRawNode("127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	defer node.close()
	mgr := dummyMgr()
	node.connect(mgr)

	replyChan := sendTestMessage(t, node, 1, callOptions{})
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		// Any response (including error) is acceptable for this test
		return true
	})
}

func TestChannelSuccessfulConnection(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()
	mgr := dummyMgr()
	defer mgr.Close()
	node, err := NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}
	if len(mgr.Nodes()) < 1 {
		t.Fatal("the node was not added to the configuration")
	}
	if !node.channel.isConnected() {
		t.Fatal("a connection could not be made to a live node")
	}
	if node.conn == nil {
		t.Fatal("connection should not be nil")
	}
}

func TestChannelUnsuccessfulConnection(t *testing.T) {
	mgr := dummyMgr()
	defer mgr.Close()
	// no servers are listening on the given address
	node, err := NewRawNode("127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}

	// the node should still be added to the configuration
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}
	if len(mgr.Nodes()) < 1 {
		t.Fatal("the node was not added to the configuration")
	}
	if node.conn == nil {
		t.Fatal("connection should not be nil")
	}
}

func TestChannelReconnection(t *testing.T) {
	srvAddr := "127.0.0.1:5000"
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())
	node, err := NewRawNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer node.close()
	mgr := dummyMgr()
	node.connect(mgr)

	// send message when server is down
	replyChan := sendTestMessage(t, node, 1, callOptions{})
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		if resp.err == nil {
			t.Error("response err: got <nil>, want error")
		}
		return true
	})

	startServer()

	// send message when server is up but not yet connected,
	// with retries to accommodate gRPC connection establishment
	var successfulSend bool
	for range 10 {
		replyChan = sendTestMessage(t, node, 2, getCallOptions(E_Multicast, nil))
		if expectResponse(t, replyChan, func(t *testing.T, resp response) bool { return resp.err == nil }) {
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
	replyChan = sendTestMessage(t, node, 3, callOptions{})
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		if resp.err == nil {
			t.Error("response err: got <nil>, want error")
		}
		return true
	})
}
