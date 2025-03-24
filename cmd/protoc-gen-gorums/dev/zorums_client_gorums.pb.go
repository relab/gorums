// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.8.0-devel
// 	protoc            v5.29.2
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(8 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 8)
)

// ZorumsServiceClient is the client interface for the ZorumsService service.
type ZorumsServiceClient interface {
	QuorumCall(ctx context.Context, in *Request) (resp *Response, err error)
	QuorumCallPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) (resp *Response, err error)
	QuorumCallCustomReturnType(ctx context.Context, in *Request) (resp *MyResponse, err error)
	QuorumCallCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) (resp *MyResponse, err error)
	QuorumCallEmpty(ctx context.Context, in *emptypb.Empty) (resp *Response, err error)
	QuorumCallEmpty2(ctx context.Context, in *Request) (resp *emptypb.Empty, err error)
	Multicast(ctx context.Context, in *Request, opts ...gorums.CallOption)
	MulticastPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request, opts ...gorums.CallOption)
	Multicast2(ctx context.Context, in *Request, opts ...gorums.CallOption)
	Multicast3(ctx context.Context, in *Request, opts ...gorums.CallOption)
	Multicast4(ctx context.Context, in *emptypb.Empty, opts ...gorums.CallOption)
	QuorumCallAsync(ctx context.Context, in *Request) *AsyncResponse
	QuorumCallAsyncPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *AsyncResponse
	QuorumCallAsyncCustomReturnType(ctx context.Context, in *Request) *AsyncMyResponse
	QuorumCallAsyncCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *AsyncMyResponse
	QuorumCallAsync2(ctx context.Context, in *Request) *AsyncResponse
	QuorumCallAsyncEmpty(ctx context.Context, in *Request) *AsyncEmpty
	QuorumCallAsyncEmpty2(ctx context.Context, in *emptypb.Empty) *AsyncResponse
	Correctable(ctx context.Context, in *Request) *CorrectableResponse
	CorrectablePerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableResponse
	CorrectableCustomReturnType(ctx context.Context, in *Request) *CorrectableMyResponse
	CorrectableCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableMyResponse
	CorrectableEmpty(ctx context.Context, in *Request) *CorrectableEmpty
	CorrectableEmpty2(ctx context.Context, in *emptypb.Empty) *CorrectableResponse
	CorrectableStream(ctx context.Context, in *Request) *CorrectableStreamResponse
	CorrectableStreamPerNodeArg(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamResponse
	CorrectableStreamCustomReturnType(ctx context.Context, in *Request) *CorrectableStreamMyResponse
	CorrectableStreamCombo(ctx context.Context, in *Request, f func(*Request, uint32) *Request) *CorrectableStreamMyResponse
	CorrectableStreamEmpty(ctx context.Context, in *Request) *CorrectableStreamEmpty
	CorrectableStreamEmpty2(ctx context.Context, in *emptypb.Empty) *CorrectableStreamResponse
}

// enforce interface compliance
var _ ZorumsServiceClient = (*ZorumsServiceConfiguration)(nil)

// ZorumsServiceNodeClient is the single node client interface for the ZorumsService service.
type ZorumsServiceNodeClient interface {
	GRPCCall(ctx context.Context, in *Request) (resp *Response, err error)
	Unicast(ctx context.Context, in *Request, opts ...gorums.CallOption)
	Unicast2(ctx context.Context, in *Request, opts ...gorums.CallOption)
}

// enforce interface compliance
var _ ZorumsServiceNodeClient = (*ZorumsServiceNode)(nil)
