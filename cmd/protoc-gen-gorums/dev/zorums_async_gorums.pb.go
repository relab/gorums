// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallFuture plain.
func (c *Configuration) QuorumCallFuture(ctx context.Context, in *Request) *FutureResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFutureMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFutureQF(req.(*Request), r)
		return result, quorum
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureResponse{fut}
}

// QuorumCallFuturePerNodeArg with per_node_arg option.
func (c *Configuration) QuorumCallFuturePerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *FutureResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFuturePerNodeArgMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFuturePerNodeArgQF(req.(*Request), r)
		return result, quorum
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureResponse{fut}
}

// QuorumCallFutureCustomReturnType with custom_return_type option.
func (c *Configuration) QuorumCallFutureCustomReturnType(ctx context.Context, in *Request) *FutureMyResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFutureCustomReturnTypeMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFutureCustomReturnTypeQF(req.(*Request), r)
		return result, quorum
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureMyResponse{fut}
}

// QuorumCallFutureCombo with all supported options.
func (c *Configuration) QuorumCallFutureCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *FutureMyResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFutureComboMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFutureComboQF(req.(*Request), r)
		return result, quorum
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureMyResponse{fut}
}

// QuorumCallFuture2 plain; with same return type: Response.
func (c *Configuration) QuorumCallFuture2(ctx context.Context, in *Request) *FutureResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFuture2MethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFuture2QF(req.(*Request), r)
		return result, quorum
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureResponse{fut}
}

// QuorumCallFutureEmpty for testing imported message type.
func (c *Configuration) QuorumCallFutureEmpty(ctx context.Context, in *Request) *FutureEmpty {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFutureEmptyMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*empty.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*empty.Empty)
		}
		result, quorum := c.qspec.QuorumCallFutureEmptyQF(req.(*Request), r)
		return result, quorum
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureEmpty{fut}
}

// QuorumCallFutureEmpty2 for testing imported message type; with same return
// type as QuorumCallFuture: Response.
func (c *Configuration) QuorumCallFutureEmpty2(ctx context.Context, in *empty.Empty) *FutureResponse {
	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: quorumCallFutureEmpty2MethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		result, quorum := c.qspec.QuorumCallFutureEmpty2QF(req.(*empty.Empty), r)
		return result, quorum
	}

	fut := gorums.FutureCall(ctx, cd)
	return &FutureResponse{fut}
}
