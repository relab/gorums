// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: zorums.proto

package dev

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

// Multicast with broadcast option enables the server handler to broadcast.
// The handler still works like a regular QuorumCall from the client's
// perpective.
func (c *Configuration) MulticastWithBroadcast(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.MulticastWithBroadcast",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// Multicast plain. Response type is not needed here.
func (c *Configuration) Multicast(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.Multicast",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// MulticastPerNodeArg with per_node_arg option.
func (c *Configuration) MulticastPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.MulticastPerNodeArg",
	}

	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// Multicast2 is testing whether multiple streams work.
func (c *Configuration) Multicast2(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.Multicast2",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

// Multicast3 is testing imported message type.
func (c *Configuration) Multicast3(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.Multicast3",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

// Multicast4 is testing imported message type.
func (c *Configuration) Multicast4(ctx context.Context, in *empty.Empty, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.Multicast4",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}
