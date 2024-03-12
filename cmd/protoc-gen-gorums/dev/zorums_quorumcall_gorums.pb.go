// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: zorums.proto

package dev

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	uuid "github.com/google/uuid"
	gorums "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// QuorumCall plain.
func (c *Configuration) QuorumCall(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCall",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// QuorumCall with per_node_arg option.
func (c *Configuration) QuorumCallPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallPerNodeArg",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallPerNodeArgQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// QuorumCall with custom_return_type option.
func (c *Configuration) QuorumCallCustomReturnType(ctx context.Context, in *Request) (resp *MyResponse, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallCustomReturnType",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallCustomReturnTypeQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*MyResponse), err
}

// QuorumCallCombo with all supported options.
func (c *Configuration) QuorumCallCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) (resp *MyResponse, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallCombo",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallComboQF(req.(*Request), r)
	}
	cd.PerNodeArgFn = func(req protoreflect.ProtoMessage, nid uint32) protoreflect.ProtoMessage {
		return f(req.(*Request), nid)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*MyResponse), err
}

// QuorumCallEmpty for testing imported message type.
func (c *Configuration) QuorumCallEmpty(ctx context.Context, in *empty.Empty) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallEmpty",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallEmptyQF(req.(*empty.Empty), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// QuorumCallEmpty2 for testing imported message type.
func (c *Configuration) QuorumCallEmpty2(ctx context.Context, in *Request) (resp *empty.Empty, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallEmpty2",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*empty.Empty, len(replies))
		for k, v := range replies {
			r[k] = v.(*empty.Empty)
		}
		return c.qspec.QuorumCallEmpty2QF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*empty.Empty), err
}

// QuorumCall with broadcast option enables the server handler to broadcast.
// The handler still works like a regular QuorumCall from the client's
// perpective.
func (c *Configuration) QuorumCallWithBroadcast(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "dev.ZorumsService.QuorumCallWithBroadcast",

		BroadcastID: uuid.New().String(),
		SenderType:  gorums.BroadcastClient,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallWithBroadcastQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}
