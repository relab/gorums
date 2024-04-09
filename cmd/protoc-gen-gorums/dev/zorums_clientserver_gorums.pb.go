// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// clientServer is the client server API for the ZorumsService Service
type clientServer interface {
	clientBroadcastWithClientHandler1(ctx context.Context, request *Response) (*Response, error)
	clientBroadcastWithClientHandler2(ctx context.Context, request *Response) (*Response, error)
	clientBroadcastWithClientHandlerAndBroadcastOption(ctx context.Context, request *Response) (*Response, error)
}

var clientServer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "protos.ClientServer",
	HandlerType: (*clientServer)(nil),
	Methods: []grpc.MethodDesc{

		{
			MethodName: "ClientBroadcastWithClientHandler1",
			Handler:    _clientBroadcastWithClientHandler1,
		},
		{
			MethodName: "ClientBroadcastWithClientHandler2",
			Handler:    _clientBroadcastWithClientHandler2,
		},
		{
			MethodName: "ClientBroadcastWithClientHandlerAndBroadcastOption",
			Handler:    _clientBroadcastWithClientHandlerAndBroadcastOption,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "",
}