package metadata

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	status "google.golang.org/grpc/status"
)

type testSrv struct{}

func (srv testSrv) IDFromMD(ctx context.Context, _ *empty.Empty, ret func(*NodeID, error)) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		ret(nil, status.Error(codes.NotFound, "Metadata unavailable"))
		return
	}
	v := md.Get("id")
	if len(v) < 1 {
		ret(nil, status.Error(codes.NotFound, "ID field missing"))
		return
	}
	id, err := strconv.Atoi(v[0])
	if err != nil {
		ret(nil, status.Errorf(codes.InvalidArgument, "Got '%s', but could not convert to integer", v[0]))
		return
	}
	ret(&NodeID{ID: uint32(id)}, nil)
}

func (srv testSrv) WhatIP(ctx context.Context, _ *empty.Empty, ret func(*IPAddr, error)) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		ret(nil, status.Error(codes.NotFound, "Peer info unavailable"))
		return
	}
	ret(&IPAddr{Addr: peerInfo.Addr.String()}, nil)
}

func initServer(t *testing.T) *gorums.Server {
	srv := gorums.NewServer()
	RegisterMetadataTestServer(srv, &testSrv{})
	return srv
}

func TestMetadata(t *testing.T) {
	want := uint32(1)

	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface { return initServer(t) })
	defer teardown()

	md := metadata.New(map[string]string{
		"id": fmt.Sprint(want),
	})

	mgr := NewManager(
		gorums.WithMetadata(md),
		gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	_, err := mgr.NewConfiguration(nil, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
	resp, err := node.IDFromMD(context.Background(), &empty.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if resp.GetID() != want {
		t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), want)
	}
}

func TestPerNodeMetadata(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 2, func(_ int) gorums.ServerIface { return initServer(t) })
	defer teardown()

	perNodeMD := func(nid uint32) metadata.MD {
		return metadata.New(map[string]string{
			"id": fmt.Sprintf("%d", nid),
		})
	}

	mgr := NewManager(
		gorums.WithPerNodeMetadata(perNodeMD),
		gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	_, err := mgr.NewConfiguration(nil, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	for _, node := range mgr.Nodes() {
		resp, err := node.IDFromMD(context.Background(), &empty.Empty{})
		if err != nil {
			t.Fatalf("RPC error: %v", err)
		}

		if resp.GetID() != node.ID() {
			t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), node.ID())
		}
	}
}

func TestCanGetPeerInfo(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface { return initServer(t) })
	defer teardown()

	mgr := NewManager(
		gorums.WithDialTimeout(1*time.Second),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	_, err := mgr.NewConfiguration(nil, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
	ip, err := node.WhatIP(context.Background(), &empty.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if ip.GetAddr() == "" {
		t.Fatalf("WhatIP() == '%s', want non-empty string", ip.GetAddr())
	}
}
