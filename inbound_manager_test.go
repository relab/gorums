package gorums

import (
	"context"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
)

// inboundTestNode is a minimal NodeAddress for use in InboundManager tests.
type inboundTestNode struct {
	addr string
}

func (n inboundTestNode) Addr() string { return n.addr }

// Compile-time assertion: nodeMap[T] implements InboundNodeOption.
var _ InboundNodeOption = nodeMap[inboundTestNode](nil)

func TestNewInboundManager(t *testing.T) {
	tests := []struct {
		name    string
		opt     InboundNodeOption
		wantIDs []uint32
		wantErr string
	}{
		{
			name: "ValidNodes",
			opt: nodeMap[inboundTestNode]{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9082"},
				3: {addr: "127.0.0.1:9083"},
			},
			wantIDs: []uint32{1, 2, 3},
		},
		{
			name:    "EmptyMapRejected",
			opt:     nodeMap[inboundTestNode]{},
			wantErr: "missing required node map",
		},
		{
			name: "NodeZeroRejected",
			opt: nodeMap[inboundTestNode]{
				0: {addr: "127.0.0.1:9080"},
				1: {addr: "127.0.0.1:9081"},
			},
			wantErr: "node 0 is reserved",
		},
		{
			name: "DuplicateAddressRejected",
			opt: nodeMap[inboundTestNode]{
				1: {addr: "127.0.0.1:9081"},
				2: {addr: "127.0.0.1:9081"}, // same address as ID 1
			},
			wantErr: "already in use by node",
		},
		{
			name: "InvalidAddressRejected",
			opt: nodeMap[inboundTestNode]{
				1: {addr: "not-an-address"},
			},
			wantErr: "invalid address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			im, err := NewInboundManager(tc.opt)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewInboundManager() error = nil; want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewInboundManager() error = %q; want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewInboundManager() unexpected error: %v", err)
			}
			got := im.KnownIDs()
			if !slices.Equal(got, tc.wantIDs) {
				t.Errorf("KnownIDs() = %v; want %v", got, tc.wantIDs)
			}
		})
	}
}

// TestInboundNodeOptionNodeListRejected verifies at runtime that nodeList
// does NOT implement InboundNodeOption. WithNodeList is intentionally excluded
// because inbound peer tracking requires pre-assigned IDs.
func TestInboundNodeOptionNodeListRejected(t *testing.T) {
	var opt NodeListOption = WithNodeList([]string{"127.0.0.1:9080"})
	if _, ok := opt.(InboundNodeOption); ok {
		t.Error("nodeList must not implement InboundNodeOption")
	}
}

// nodeIDCtx builds a context carrying incoming metadata with a gorums-node-id value.
func nodeIDCtx(id string) context.Context {
	md := metadata.Pairs(gorumsNodeIDKey, id)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestNodeID(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		wantID uint32
	}{
		{name: "ValidID", ctx: nodeIDCtx("3"), wantID: 3},
		{name: "ValidIDOne", ctx: nodeIDCtx("1"), wantID: 1},
		{name: "ValidIDLarge", ctx: nodeIDCtx("4294967295"), wantID: 4294967295}, // max uint32
		{name: "ExternalClientNoMeta", ctx: context.Background(), wantID: 0},
		{name: "ReservedIDZero", ctx: nodeIDCtx("0"), wantID: 0},
		{name: "NegativeValue", ctx: nodeIDCtx("-1"), wantID: 0},
		{name: "NonNumericValue", ctx: nodeIDCtx("abc"), wantID: 0},
		{name: "Overflow", ctx: nodeIDCtx("4294967296"), wantID: 0}, // max uint32 + 1
		{name: "EmptyString", ctx: nodeIDCtx(""), wantID: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nodeID(tc.ctx); got != tc.wantID {
				t.Errorf("nodeID() = %d; want %d", got, tc.wantID)
			}
		})
	}
}
