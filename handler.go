package gorums

import (
	"context"

	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type implementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)

func DefaultHandler[T RequestTypes, V ResponseTypes](impl implementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

type broadcastMsg interface {
	GetFrom() string
	GetRequest() RequestTypes
	GetMethod() string
	GetContext() ServerCtx
}

type broadcastMessage[T RequestTypes, V ResponseTypes] struct {
	from           string
	request        T
	implementation implementationFunc[T, V]
	method         string
	context        ServerCtx
}

func (b *broadcastMessage[T, V]) GetFrom() string {
	return b.from
}

func (b *broadcastMessage[T, V]) GetRequest() RequestTypes {
	return b.request
}

func (b *broadcastMessage[T, V]) GetImplementation() implementationFunc[T, V] {
	return b.implementation
}

func (b *broadcastMessage[T, V]) GetMethod() string {
	return b.method
}

func (b *broadcastMessage[T, V]) GetContext() ServerCtx {
	return b.context
}

func newBroadcastMessage[T RequestTypes, V ResponseTypes](ctx ServerCtx, req T, impl implementationFunc[T, V], method string) *broadcastMessage[T, V] {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage[T, V]{
		from:           addr,
		request:        req,
		implementation: impl,
		method:         method,
		context:        ctx,
	}
}

func BestEffortBroadcastHandler[T RequestTypes, V ResponseTypes](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		if srv.shouldBroadcast(in.Metadata.MessageID) {
			// need to know:
			//	- which method to call on the other servers
			//	- who sent the message
			//	- the type of the message
			//broadcastChan <- newBroadcastMessage[T, V](ctx, req, impl)
			go srv.broadcast(newBroadcastMessage[T, V](ctx, req, impl, in.Metadata.Method))
		}
		var resp ResponseTypes
		var err error
		if !srv.alreadyReceivedFromPeer(ctx, in.Metadata.MessageID) {
			resp, err = impl(ctx, req)
		}
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func (srv *Server) shouldBroadcast(msgId uint64) bool {
	_, ok := srv.recievedFrom[msgId]
	if !ok {
		srv.recievedFrom[msgId] = make(map[string]bool)
		return true
	}
	return false
}

func (srv *Server) alreadyReceivedFromPeer(ctx ServerCtx, msgId uint64) bool {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	receivedMsgFromNode, ok := srv.recievedFrom[msgId][addr]
	if !ok {
		if !receivedMsgFromNode {
			srv.recievedFrom[msgId][addr] = true
			return false
		}
	}
	return true

}

func (srv *Server) broadcast(broadcastMessage broadcastMsg) {
	srv.BroadcastChan <- broadcastMessage
}

func RegisterBroadcastFunc[T RequestTypes, V ResponseTypes](impl func(context.Context, T) (V, error)) func(context.Context, any) (any, error) {
	return func(ctx context.Context, req any) (resp any, err error) {
		return impl(ctx, req.(T))
	}
}
