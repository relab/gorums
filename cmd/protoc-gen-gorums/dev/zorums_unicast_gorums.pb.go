// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
<<<<<<< HEAD
// 	protoc            v4.24.4
=======
// 	protoc            v3.12.4
>>>>>>> 0da29c48 (init code gen)
// source: zorums.proto

package dev

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
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
var _ empty.Empty

// Unicast2 is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (n *Node) Unicast2(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.CallData{
		Message: in,
		Method:  "dev.ZorumsService.Unicast2",
	}

	n.RawNode.Unicast(ctx, cd, opts...)
}
