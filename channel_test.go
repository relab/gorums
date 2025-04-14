package gorums

import (
	"bytes"
	"context"
	"crypto/elliptic"
	"net"
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
		_ = SendMessage(ctx, finished, WrapMessage(in.Metadata, resp, err))
	})
	return srv
}

func TestChannelCreation(t *testing.T) {
	node, err := NewRawNode("127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	// the node should be closed manually because it isn't added to the configuration
	defer node.close()
	mgr := dummyMgr()
	// a proper connection should NOT be established here
	_ = node.connect(mgr)

	replyChan := make(chan response, 1)
	go func() {
		ctx := context.Background()
		md := ordering.NewGorumsMetadata(ctx, 1, handlerName)
		req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}}
		node.channel.enqueue(req, replyChan, false)
	}()
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
	srvAddr := "127.0.0.1:5005"
	// wait to start the server
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())
	node, err := NewRawNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	// the node should be closed manually because it isn't added to the configuration
	defer node.close()
	mgr := dummyMgr()
	// a proper connection should NOT be established here because server is not started
	_ = node.connect(mgr)

	// send first message when server is down
	replyChan1 := make(chan response, 1)
	go func() {
		ctx := context.Background()
		md := ordering.NewGorumsMetadata(ctx, 1, handlerName)
		req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}}
		node.channel.enqueue(req, replyChan1, false)
	}()

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
	replyChan2 := make(chan response, 1)
	go func() {
		ctx := context.Background()
		md := ordering.NewGorumsMetadata(ctx, 2, handlerName)
		req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}, opts: getCallOptions(E_Multicast, nil)}
		node.channel.enqueue(req, replyChan2, false)
	}()

	// check response: error should be nil because server is up
	select {
	case resp := <-replyChan2:
		if resp.err != nil {
			t.Errorf("response err: got %v, want <nil>", resp.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: impossible to enqueue messages to the node")
	}

	stopServer()

	// send third message when server has been previously up but is now down
	replyChan3 := make(chan response, 1)
	go func() {
		ctx := context.Background()
		md := ordering.NewGorumsMetadata(ctx, 3, handlerName)
		req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}}
		node.channel.enqueue(req, replyChan3, false)
	}()

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

func TestAuthentication(t *testing.T) {
	srv := NewServer(EnforceAuthentication(elliptic.P256()))
	node, err := NewRawNode("127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuth(elliptic.P256())
	_ = auth.GenerateKeys()
	privKey, pubKey := auth.Keys()
	auth.RegisterKeys(addr, privKey, pubKey)
	mgr := NewRawManager(WithAuthentication(auth))
	defer mgr.Close()
	node.mgr = mgr
	config := make(RawConfiguration, 0)
	config = append(config, node)

	ctx := context.Background()
	md := ordering.NewGorumsMetadata(ctx, 0, "test")
	broadcastMsg := ordering.BroadcastMsg_builder{
		IsBroadcastClient: true,
		BroadcastID:       0,
		SenderAddr:        "127.0.0.1:5000",
		OriginAddr:        "127.0.0.1:5000",
		OriginMethod:      "test",
	}.Build()
	md.SetBroadcastMsg(broadcastMsg)
	msg := &Message{Metadata: md, Message: &mock.Request{}}
	msg1 := &Message{Metadata: md, Message: &mock.Request{}}

	chEncodedMsg := config.encodeMsg(msg1)
	srvEncodedMsg := msg1.Encode()
	if !bytes.Equal(chEncodedMsg, srvEncodedMsg) {
		t.Fatalf("wrong encoding. want: %x, got: %x", chEncodedMsg, srvEncodedMsg)
	}
	pemEncodedPub, err := auth.EncodePublic()
	if err != nil {
		t.Fatal(err)
	}
	signature, err := auth.Sign(chEncodedMsg)
	if err != nil {
		t.Fatal(err)
	}
	err = auth.VerifySignature(pemEncodedPub, chEncodedMsg, signature)
	if err != nil {
		// channel encoded msg not valid
		t.Fatalf("VerifySignature() = %v, want nil", err)
	}
	err = auth.VerifySignature(pemEncodedPub, srvEncodedMsg, signature)
	if err != nil {
		// srv encoded msg not valid
		t.Fatalf("VerifySignature() = %v, want nil", err)
	}

	config.sign(msg)

	if pemEncodedPub != msg.Metadata.GetAuthMsg().GetPublicKey() {
		t.Fatalf("wrong pub key. want: %s, got: %s", pemEncodedPub, msg.Metadata.GetAuthMsg().GetPublicKey())
	}

	if err := srv.srv.verify(msg); err != nil {
		t.Fatalf("authentication failed. want: nil, got: %s", err)
	}
}
