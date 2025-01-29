// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
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
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// Correctable plain.
func (c *Configuration) Correctable(ctx context.Context, in *Request) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.Correctable",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectablePerNodeArg with per_node_arg option.
func (c *Configuration) CorrectablePerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectablePerNodeArg",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectablePerNodeArgQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectableCustomReturnType with custom_return_type option.
func (c *Configuration) CorrectableCustomReturnType(ctx context.Context, in *Request) *CorrectableMyResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableCustomReturnType",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableCustomReturnTypeQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableMyResponse{corr}
}

// CorrectableCombo with all supported options.
func (c *Configuration) CorrectableCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableMyResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableCombo",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableComboQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableMyResponse{corr}
}

// CorrectableEmpty for testing imported message type.
func (c *Configuration) CorrectableEmpty(ctx context.Context, in *Request) *CorrectableEmpty {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableEmpty",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*emptypb.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*emptypb.Empty)
		}
		return c.qspec.CorrectableEmptyQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableEmpty{corr}
}

// CorrectableEmpty2 for testing imported message type; with same return
// type as Correctable: Response.
func (c *Configuration) CorrectableEmpty2(ctx context.Context, in *emptypb.Empty) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableEmpty2",
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableEmpty2QF(req.(*emptypb.Empty), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectableStream plain.
func (c *Configuration) CorrectableStream(ctx context.Context, in *Request) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStream",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}

// CorrectablePerNodeArg with per_node_arg option.
func (c *Configuration) CorrectableStreamPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStreamPerNodeArg",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamPerNodeArgQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}

// CorrectableCustomReturnType with custom_return_type option.
func (c *Configuration) CorrectableStreamCustomReturnType(ctx context.Context, in *Request) *CorrectableStreamMyResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStreamCustomReturnType",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamCustomReturnTypeQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamMyResponse{corr}
}

// CorrectableCombo with all supported options.
func (c *Configuration) CorrectableStreamCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamMyResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStreamCombo",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamComboQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamMyResponse{corr}
}

// CorrectableEmpty for testing imported message type.
func (c *Configuration) CorrectableStreamEmpty(ctx context.Context, in *Request) *CorrectableStreamEmpty {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStreamEmpty",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*emptypb.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*emptypb.Empty)
		}
		return c.qspec.CorrectableStreamEmptyQF(req.(*Request), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamEmpty{corr}
}

// CorrectableEmpty2 for testing imported message type; with same return
// type as Correctable: Response.
func (c *Configuration) CorrectableStreamEmpty2(ctx context.Context, in *emptypb.Empty) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Message:      in,
		Method:       "dev.ZorumsService.CorrectableStreamEmpty2",
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamEmpty2QF(req.(*emptypb.Empty), r)
	}

	corr := c.RawConfiguration.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}
