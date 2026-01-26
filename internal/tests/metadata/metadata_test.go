package metadata

import (
	"fmt"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/strconv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type testSrv struct{}

func (testSrv) IDFromMD(ctx gorums.ServerCtx, _ *emptypb.Empty) (resp *NodeIDMsg, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.NotFound, "metadata unavailable")
	}
	v := md.Get("id")
	if len(v) < 1 {
		return nil, status.Error(codes.NotFound, "missing metadata field: id")
	}
	id, err := strconv.ParseInteger[uint32](v[0], 10)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "value of id field: %q is not a number: %v", v[0], err)
	}
	return NodeIDMsg_builder{ID: id}.Build(), nil
}

func (testSrv) WhatIP(ctx gorums.ServerCtx, _ *emptypb.Empty) (resp *IPAddr, err error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.NotFound, "Peer info unavailable")
	}
	return IPAddr_builder{Addr: peerInfo.Addr.String()}.Build(), nil
}

func serverFn(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	RegisterMetadataTestServer(srv, &testSrv{})
	return srv
}

func TestMetadata(t *testing.T) {
	want := uint32(1)
	md := metadata.New(map[string]string{
		"id": fmt.Sprint(want),
	})

	node := gorums.TestNode(t, serverFn, gorums.WithMetadata(md))
	nodeCtx := node.Context(t.Context())
	resp, err := IDFromMD(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if resp.GetID() != want {
		t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), want)
	}
}

func TestPerMessageMetadata(t *testing.T) {
	node := gorums.TestNode(t, serverFn)

	want := uint32(1)
	md := metadata.New(map[string]string{
		"id": fmt.Sprint(want),
	})
	ctx := metadata.NewOutgoingContext(t.Context(), md)
	nodeCtx := node.Context(ctx)
	resp, err := IDFromMD(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if resp.GetID() != want {
		t.Fatalf("IDFromMD() == %d, want %d", resp.GetID(), want)
	}
}

func TestPerNodeMetadata(t *testing.T) {
	perNodeMD := func(nid uint32) metadata.MD {
		return metadata.New(map[string]string{
			"id": fmt.Sprintf("%d", nid),
		})
	}

	cfg := gorums.TestConfiguration(t, 2, serverFn, gorums.WithPerNodeMetadata(perNodeMD))

	for _, node := range cfg {
		nodeCtx := node.Context(t.Context())
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
	node := gorums.TestNode(t, serverFn)
	nodeCtx := node.Context(t.Context())
	ip, err := WhatIP(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if ip.GetAddr() == "" {
		t.Fatalf("WhatIP() == '%s', want non-empty string", ip.GetAddr())
	}
}
