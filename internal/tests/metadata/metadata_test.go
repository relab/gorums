package metadata

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type testSrv struct{}

func (srv testSrv) IDFromMD(ctx gorums.ServerCtx, _ *emptypb.Empty) (resp *NodeID, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.NotFound, "metadata unavailable")
	}
	v := md.Get("id")
	if len(v) < 1 {
		return nil, status.Error(codes.NotFound, "missing metadata field: id")
	}
	id, err := strconv.Atoi(v[0])
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "value of id field: %q is not a number: %v", v[0], err)
	}
	return NodeID_builder{ID: uint32(id)}.Build(), nil
}

func (srv testSrv) WhatIP(ctx gorums.ServerCtx, _ *emptypb.Empty) (resp *IPAddr, err error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.NotFound, "Peer info unavailable")
	}
	return IPAddr_builder{Addr: peerInfo.Addr.String()}.Build(), nil
}

func initServer() *gorums.Server {
	srv := gorums.NewServer()
	RegisterMetadataTestServer(srv, &testSrv{})
	return srv
}

func TestMetadata(t *testing.T) {
	want := uint32(1)

	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface { return initServer() })
	defer teardown()

	md := metadata.New(map[string]string{
		"id": fmt.Sprint(want),
	})

	mgr := gorums.NewManager(
		gorums.WithMetadata(md),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := cfg[0]
	nodeCtx := gorums.WithNodeContext(context.Background(), node)
	resp, err := IDFromMD(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if resp.GetID() != want {
		t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), want)
	}
}

func TestPerMessageMetadata(t *testing.T) {
	want := uint32(1)

	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface { return initServer() })
	defer teardown()

	mgr := gorums.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := cfg[0]

	md := metadata.New(map[string]string{
		"id": fmt.Sprint(want),
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	nodeCtx := gorums.WithNodeContext(ctx, node)
	resp, err := IDFromMD(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if resp.GetID() != want {
		t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), want)
	}
}

func TestPerNodeMetadata(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 2, func(_ int) gorums.ServerIface { return initServer() })
	defer teardown()

	perNodeMD := func(nid uint32) metadata.MD {
		return metadata.New(map[string]string{
			"id": fmt.Sprintf("%d", nid),
		})
	}

	mgr := gorums.NewManager(
		gorums.WithPerNodeMetadata(perNodeMD),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	for _, node := range cfg {
		nodeCtx := gorums.WithNodeContext(context.Background(), node)
		resp, err := IDFromMD(nodeCtx, &emptypb.Empty{})
		if err != nil {
			t.Fatalf("RPC error: %v", err)
		}

		if resp.GetID() != node.ID() {
			t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), node.ID())
		}
	}
}

func TestCanGetPeerInfo(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface { return initServer() })
	defer teardown()

	mgr := gorums.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := cfg[0]
	nodeCtx := gorums.WithNodeContext(context.Background(), node)
	ip, err := WhatIP(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if ip.GetAddr() == "" {
		t.Fatalf("WhatIP() == '%s', want non-empty string", ip.GetAddr())
	}
}
