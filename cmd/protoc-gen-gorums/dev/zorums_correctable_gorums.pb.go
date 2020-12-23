// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

// Correctable plain.
func (c *Configuration) Correctable(ctx context.Context, in *Request) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableMethodID,
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectablePerNodeArg with per_node_arg option.
func (c *Configuration) CorrectablePerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctablePerNodeArgMethodID,
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

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectableCustomReturnType with custom_return_type option.
func (c *Configuration) CorrectableCustomReturnType(ctx context.Context, in *Request) *CorrectableMyResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableCustomReturnTypeMethodID,
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableCustomReturnTypeQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableMyResponse{corr}
}

// CorrectableCombo with all supported options.
func (c *Configuration) CorrectableCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableMyResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableComboMethodID,
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

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableMyResponse{corr}
}

// CorrectableEmpty for testing imported message type.
func (c *Configuration) CorrectableEmpty(ctx context.Context, in *Request) *CorrectableEmpty {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableEmptyMethodID,
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*empty.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*empty.Empty)
		}
		return c.qspec.CorrectableEmptyQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableEmpty{corr}
}

// CorrectableEmpty2 for testing imported message type; with same return
// type as Correctable: Response.
func (c *Configuration) CorrectableEmpty2(ctx context.Context, in *empty.Empty) *CorrectableResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableEmpty2MethodID,
		ServerStream: false,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableEmpty2QF(req.(*empty.Empty), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableResponse{corr}
}

// CorrectableStream plain.
func (c *Configuration) CorrectableStream(ctx context.Context, in *Request) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamMethodID,
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}

// CorrectablePerNodeArg with per_node_arg option.
func (c *Configuration) CorrectableStreamPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamPerNodeArgMethodID,
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

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}

// CorrectableCustomReturnType with custom_return_type option.
func (c *Configuration) CorrectableStreamCustomReturnType(ctx context.Context, in *Request) *CorrectableStreamMyResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamCustomReturnTypeMethodID,
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamCustomReturnTypeQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamMyResponse{corr}
}

// CorrectableCombo with all supported options.
func (c *Configuration) CorrectableStreamCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamMyResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamComboMethodID,
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

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamMyResponse{corr}
}

// CorrectableEmpty for testing imported message type.
func (c *Configuration) CorrectableStreamEmpty(ctx context.Context, in *Request) *CorrectableStreamEmpty {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamEmptyMethodID,
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*empty.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*empty.Empty)
		}
		return c.qspec.CorrectableStreamEmptyQF(req.(*Request), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamEmpty{corr}
}

// CorrectableEmpty2 for testing imported message type; with same return
// type as Correctable: Response.
func (c *Configuration) CorrectableStreamEmpty2(ctx context.Context, in *empty.Empty) *CorrectableStreamResponse {
	cd := gorums.CorrectableCallData{
		Manager:      c.mgr.Manager,
		Nodes:        c.nodes,
		Message:      in,
		MethodID:     correctableStreamEmpty2MethodID,
		ServerStream: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.CorrectableStreamEmpty2QF(req.(*empty.Empty), r)
	}

	corr := gorums.CorrectableCall(ctx, cd)
	return &CorrectableStreamResponse{corr}
}
