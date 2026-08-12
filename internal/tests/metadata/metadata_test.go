package metadata

import (
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/strconv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type testSrv struct{}

func (testSrv) IDFromMD(ctx gorums.ServerContext, _ *emptypb.Empty) (resp *NodeID, err error) {
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
	return NodeID_builder{ID: id}.Build(), nil
}

func (testSrv) WhatIP(ctx gorums.ServerContext, _ *emptypb.Empty) (resp *IPAddr, err error) {
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

	node := gorumstest.Node(t, serverFn, gorums.WithMetadata(md))
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
	node := gorumstest.Node(t, serverFn)

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

func TestPerMessageMetadataAcrossStreamTopologies(t *testing.T) {
	tests := []struct {
		name  string
		dedup bool
	}{
		{name: "Dual"},
		{name: "Dedup", dedup: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []gorums.ServerOption
			if tt.dedup {
				opts = append(opts, gorums.WithStreamDedup())
			}
			servers := gorumstest.LocalServers(t, 2, opts...)
			for _, srv := range servers {
				RegisterMetadataTestServer(srv, &testSrv{})
			}
			if tt.dedup {
				ctx := gorumstest.Context(t, 10*time.Second)
				for _, srv := range servers {
					if _, err := srv.WaitForAll(ctx); err != nil {
						t.Fatalf("WaitForAll: %v", err)
					}
				}
			}

			var target *gorums.Node
			for _, node := range servers[1].PeerConfig() {
				if node.ID() == 1 {
					target = node
					break
				}
			}
			if target == nil {
				t.Fatal("node 1 not found in server 2 outbound configuration")
			}
			if target.IsShared() != tt.dedup {
				t.Fatalf("node 1 IsShared = %t, want %t", target.IsShared(), tt.dedup)
			}

			const want = uint32(42)
			ctx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("id", fmt.Sprint(want)))
			resp, err := IDFromMD(target.Context(ctx), &emptypb.Empty{})
			if err != nil {
				t.Fatalf("IDFromMD: %v", err)
			}
			if got := resp.GetID(); got != want {
				t.Fatalf("IDFromMD() = %d, want %d", got, want)
			}
		})
	}
}

func TestCanGetPeerInfo(t *testing.T) {
	node := gorumstest.Node(t, serverFn)
	nodeCtx := node.Context(t.Context())
	ip, err := WhatIP(nodeCtx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if ip.GetAddr() == "" {
		t.Fatalf("WhatIP() == '%s', want non-empty string", ip.GetAddr())
	}
}
