// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.9.0-devel+d62f9a0e
// 	protoc            v5.29.3
// source: zorums.proto

package dev

import (
	gorums "github.com/relab/gorums"
	ordering "github.com/relab/gorums/ordering"
	proto "google.golang.org/protobuf/proto"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(9 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 9)
)

// ZorumsService is the server-side API for the ZorumsService Service
type ZorumsServiceServer interface {
	GRPCCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallEmpty(ctx gorums.ServerCtx, request *emptypb.Empty) (response *Response, err error)
	QuorumCallEmpty2(ctx gorums.ServerCtx, request *Request) (response *emptypb.Empty, err error)
	QuorumCallWithBroadcast(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	MulticastWithBroadcast(ctx gorums.ServerCtx, request *Request)
	BroadcastInternal(ctx gorums.ServerCtx, request *Request) (response *emptypb.Empty, err error)
	BroadcastWithClientHandler1(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	BroadcastWithClientHandler2(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	BroadcastWithClientHandlerAndBroadcastOption(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	Multicast(ctx gorums.ServerCtx, request *Request)
	MulticastPerNodeArg(ctx gorums.ServerCtx, request *Request)
	Multicast2(ctx gorums.ServerCtx, request *Request)
	Multicast3(ctx gorums.ServerCtx, request *Request)
	Multicast4(ctx gorums.ServerCtx, request *emptypb.Empty)
	QuorumCallAsync(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsync2(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncEmpty(ctx gorums.ServerCtx, request *Request) (response *emptypb.Empty, err error)
	QuorumCallAsyncEmpty2(ctx gorums.ServerCtx, request *emptypb.Empty) (response *Response, err error)
	Correctable(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectablePerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableEmpty(ctx gorums.ServerCtx, request *Request) (response *emptypb.Empty, err error)
	CorrectableEmpty2(ctx gorums.ServerCtx, request *emptypb.Empty) (response *Response, err error)
	CorrectableStream(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamPerNodeArg(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamCustomReturnType(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamCombo(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamEmpty(ctx gorums.ServerCtx, request *Request, send func(response *emptypb.Empty) error) error
	CorrectableStreamEmpty2(ctx gorums.ServerCtx, request *emptypb.Empty, send func(response *Response) error) error
	Unicast(ctx gorums.ServerCtx, request *Request)
	Unicast2(ctx gorums.ServerCtx, request *Request)
}

func RegisterZorumsServiceServer(srv *gorums.Server, impl ZorumsServiceServer) {
	srv.RegisterHandler("dev.ZorumsService.GRPCCall", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.GRPCCall(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCall", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCall(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallPerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		resp, err := impl.QuorumCallEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallWithBroadcast", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallWithBroadcast(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.MulticastWithBroadcast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.MulticastWithBroadcast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.BroadcastInternal", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.BroadcastInternal(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.BroadcastWithClientHandler1", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.BroadcastWithClientHandler1(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.BroadcastWithClientHandler2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.BroadcastWithClientHandler2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.BroadcastWithClientHandlerAndBroadcastOption", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.BroadcastWithClientHandlerAndBroadcastOption(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.MulticastPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.MulticastPerNodeArg(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast2", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast2(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast3", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast3(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast4", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		impl.Multicast4(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsync(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncPerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsync2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.Correctable", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.Correctable(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectablePerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectablePerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		resp, err := impl.CorrectableEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStream", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStream(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamPerNodeArg(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamCustomReturnType(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamCombo(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamEmpty(ctx, req, func(resp *emptypb.Empty) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		err := impl.CorrectableStreamEmpty2(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Unicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast2", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Unicast2(ctx, req)
	})
}
