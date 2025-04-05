// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.9.0-devel+903a0213
// 	protoc            v5.29.3
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(9 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 9)
)

// GRPCCall plain gRPC call; testing that Gorums can ignore these, but that
// they are added to the _grpc.pb.go generated file.
func (n *Node) GRPCCall(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "dev.ZorumsService.GRPCCall",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}
