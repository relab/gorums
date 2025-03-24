// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.8.0-devel
// 	protoc            v5.29.2
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(8 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 8)
)

// QuorumCallAsync plain.
func (c *ZorumsServiceConfiguration) QuorumCallAsync(ctx context.Context, in *Request) *AsyncResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsync",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsyncQF(req.(*Request), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncResponse{fut}
}

// QuorumCallAsyncPerNodeArg with per_node_arg option.
func (c *ZorumsServiceConfiguration) QuorumCallAsyncPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *AsyncResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsyncPerNodeArg",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsyncPerNodeArgQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncResponse{fut}
}

// QuorumCallAsyncCustomReturnType with custom_return_type option.
func (c *ZorumsServiceConfiguration) QuorumCallAsyncCustomReturnType(ctx context.Context, in *Request) *AsyncMyResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsyncCustomReturnType",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsyncCustomReturnTypeQF(req.(*Request), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncMyResponse{fut}
}

// QuorumCallAsyncCombo with all supported options.
func (c *ZorumsServiceConfiguration) QuorumCallAsyncCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *AsyncMyResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsyncCombo",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsyncComboQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncMyResponse{fut}
}

// QuorumCallAsync2 plain; with same return type: Response.
func (c *ZorumsServiceConfiguration) QuorumCallAsync2(ctx context.Context, in *Request) *AsyncResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsync2",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsync2QF(req.(*Request), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncResponse{fut}
}

// QuorumCallAsyncEmpty for testing imported message type.
func (c *ZorumsServiceConfiguration) QuorumCallAsyncEmpty(ctx context.Context, in *Request) *AsyncEmpty {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsyncEmpty",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*emptypb.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*emptypb.Empty)
		}
		return c.qspec.QuorumCallAsyncEmptyQF(req.(*Request), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncEmpty{fut}
}

// QuorumCallAsyncEmpty2 for testing imported message type; with same return
// type as QuorumCallAsync: Response.
func (c *ZorumsServiceConfiguration) QuorumCallAsyncEmpty2(ctx context.Context, in *emptypb.Empty) *AsyncResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallAsyncEmpty2",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallAsyncEmpty2QF(req.(*emptypb.Empty), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncResponse{fut}
}
