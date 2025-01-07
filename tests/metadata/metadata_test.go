package metadata

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

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
		return nil, status.Error(codes.NotFound, "Metadata unavailable")
	}
	v := md.Get("id")
	if len(v) < 1 {
		return nil, status.Error(codes.NotFound, "ID field missing")
	}
	id, err := strconv.Atoi(v[0])
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Got '%s', but could not convert to integer", v[0])
	}
	return &NodeID{ID: uint32(id)}, nil
}

func (srv testSrv) WhatIP(ctx gorums.ServerCtx, _ *emptypb.Empty) (resp *IPAddr, err error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.NotFound, "Peer info unavailable")
	}
	return &IPAddr{Addr: peerInfo.Addr.String()}, nil
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
		gorums.WithDialTimeout(time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
	resp, err := node.IDFromMD(context.Background(), &emptypb.Empty{})
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
		gorums.WithDialTimeout(time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	for _, node := range mgr.Nodes() {
		resp, err := node.IDFromMD(context.Background(), &emptypb.Empty{})
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
		gorums.WithDialTimeout(time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
	ip, err := node.WhatIP(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if ip.GetAddr() == "" {
		t.Fatalf("WhatIP() == '%s', want non-empty string", ip.GetAddr())
	}
}
