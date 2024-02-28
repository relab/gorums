package gorums_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func createBroadcastServer(ownAddr string, srvAddrs []string) error {
	srv := gorums.NewServer()

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	srv.RegisterView(ownAddr, srvAddrs)
	srv.ListenForBroadcast()
	return nil
}

func TestBroadcast(t *testing.T) {
	var message string
	signal := make(chan struct{})

	srv := gorums.NewServer(gorums.WithConnectCallback(func(ctx context.Context) {
		m, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return
		}
		message = m.Get("message")[0]
		signal <- struct{}{}
	}))

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	md := metadata.New(map[string]string{"message": "hello"})

	mgr := gorums.NewRawManager(
		gorums.WithDialTimeout(time.Second),
		gorums.WithMetadata(md),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := gorums.NewRawNode(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	select {
	case <-time.After(100 * time.Millisecond):
	case <-signal:
	}

	if message != "hello" {
		t.Errorf("incorrect message: got '%s', want 'hello'", message)
	}
}
