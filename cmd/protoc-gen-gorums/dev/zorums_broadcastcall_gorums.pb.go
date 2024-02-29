// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: zorums.proto

package dev

import (
	context "context"
	fmt "fmt"
	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

func _clientBroadcastWithClientHandler1(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Response)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(clientServer).clientBroadcastWithClientHandler1(ctx, in)
}

func (srv *clientServerImpl) clientBroadcastWithClientHandler1(ctx context.Context, resp *Response) (*Response, error) {
	err := srv.AddResponse(ctx, resp)
	return resp, err
}

func (c *Configuration) BroadcastWithClientHandler1(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined.")
	}
	doneChan, cd := c.srv.AddRequest(ctx, in, gorums.ConvertToType(c.qspec.BroadcastWithClientHandler1QF))
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*Response), err
}

func _clientBroadcastWithClientHandler2(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClientResponse)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(clientServer).clientBroadcastWithClientHandler2(ctx, in)
}

func (srv *clientServerImpl) clientBroadcastWithClientHandler2(ctx context.Context, resp *ClientResponse) (*ClientResponse, error) {
	err := srv.AddResponse(ctx, resp)
	return resp, err
}

func (c *Configuration) BroadcastWithClientHandler2(ctx context.Context, in *Request) (resp *ClientResponse, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined.")
	}
	doneChan, cd := c.srv.AddRequest(ctx, in, gorums.ConvertToType(c.qspec.BroadcastWithClientHandler2QF))
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*ClientResponse), err
}

func _clientBroadcastWithClientHandlerAndBroadcastOption(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ClientResponse)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(clientServer).clientBroadcastWithClientHandlerAndBroadcastOption(ctx, in)
}

func (srv *clientServerImpl) clientBroadcastWithClientHandlerAndBroadcastOption(ctx context.Context, resp *ClientResponse) (*ClientResponse, error) {
	err := srv.AddResponse(ctx, resp)
	return resp, err
}

func (c *Configuration) BroadcastWithClientHandlerAndBroadcastOption(ctx context.Context, in *Request) (resp *ClientResponse, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined.")
	}
	doneChan, cd := c.srv.AddRequest(ctx, in, gorums.ConvertToType(c.qspec.BroadcastWithClientHandlerAndBroadcastOptionQF))
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*ClientResponse), err
}