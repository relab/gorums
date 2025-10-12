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
// Uses t.Context() for proper test timeout handling.
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

func TestChannelCreation(t *testing.T) {
	node, err := NewRawNode("127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	defer node.close()
	mgr := dummyMgr()
	node.connect(mgr)

	replyChan := sendTestMessage(t, node, 1, callOptions{})

	select {
	case <-replyChan:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}
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

	// send first message when server is down
	replyChan1 := sendTestMessage(t, node, 1, callOptions{})

	// check response: should be error because server is down
	select {
	case resp := <-replyChan1:
		if resp.err == nil {
			t.Error("response err: got <nil>, want error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}

	startServer()

	// send second message when server is up
	// Retry to accommodate gRPC's exponential backoff after first failure
	var successfulSend bool
	for attempt := 0; attempt < 5; attempt++ {
		replyChan2 := sendTestMessage(t, node, 2, getCallOptions(E_Multicast, nil))

		select {
		case resp := <-replyChan2:
			if resp.err == nil {
				successfulSend = true
				break
			}
			// Server is up but gRPC still in backoff, retry after delay
			if attempt < 4 {
				time.Sleep(500 * time.Millisecond)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("deadlock: impossible to enqueue messages to the node")
		}
	}
	if !successfulSend {
		t.Error("failed to send message after server came back up")
	}

	stopServer()

	// send third message when server has been previously up but is now down
	replyChan3 := sendTestMessage(t, node, 3, callOptions{})

	// check response: should be error because server is down
	select {
	case resp3 := <-replyChan3:
		if resp3.err == nil {
			t.Error("response err: got <nil>, want error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}
}
