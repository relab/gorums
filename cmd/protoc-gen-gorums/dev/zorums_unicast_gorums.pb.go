// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.9.0-devel+d9c1ffe0
// 	protoc            v5.29.3
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(9 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 9)
)

// Unicast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (n *Node) Unicast(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.CallData{
		Message: in,
		Method:  "dev.ZorumsService.Unicast",
	}

	n.RawNode.Unicast(ctx, cd, opts...)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ emptypb.Empty

// Unicast2 is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (n *Node) Unicast2(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.CallData{
		Message: in,
		Method:  "dev.ZorumsService.Unicast2",
	}

	n.RawNode.Unicast(ctx, cd, opts...)
}
