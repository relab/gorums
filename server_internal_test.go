package gorums

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestServerCallback(t *testing.T) {
	var message string
	signal := make(chan struct{})

	srv := NewServer(WithConnectCallback(func(ctx context.Context) {
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

	mgr := newManager(
		WithMetadata(md),
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.close()

	node, err := NewRawNode(lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.addNode(node); err != nil {
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
