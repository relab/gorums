package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func DefaultHandler[T requestTypes, V responseTypes](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func BroadcastHandler[T requestTypes, V broadcastStruct](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		//log.Println("BroadcastID:", in.Metadata.BroadcastMsg.BroadcastID, "Method:", in.Metadata.Method)
		srv.broadcastSrv.Lock()
		defer srv.broadcastSrv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		//var broadcast *bool = new(bool)
		//*broadcast = false
		//request := new(U)
		//resp, err := impl(ctx, req, determineBroadcast2[U](broadcast, request, srv))
		srv.broadcastSrv.runMiddleware()
		srv.broadcastSrv.b.Reset(in.Metadata.BroadcastMsg.BroadcastID)
		_ = impl(ctx, req, srv.broadcastSrv.b.(V))
		if srv.broadcastSrv.b.ShouldBroadcast() && !srv.broadcastSrv.alreadyBroadcasted(in.Metadata.BroadcastMsg.BroadcastID, srv.broadcastSrv.b.GetMethod()) {
			// how to define individual request message to each node?
			//	- maybe create one request for each node and send a list of requests?
			go srv.broadcastSrv.broadcast(newBroadcastMessage(ctx, srv.broadcastSrv.b.GetRequest(), srv.broadcastSrv.b.GetMethod(), in.Metadata.BroadcastMsg.BroadcastID))
		}
		if srv.broadcastSrv.b.ShouldReturnToClient() && !srv.broadcastSrv.alreadyReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, srv.broadcastSrv.b.GetMethod()) {
			srv.broadcastSrv.setReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, true)
			go func() {
				srv.broadcastSrv.responseChan <- newResponseMessage(srv.broadcastSrv.b.GetResponse(), srv.broadcastSrv.b.GetError(), in.Metadata.BroadcastMsg.BroadcastID, clientResponse, srv.broadcastSrv.timeout)
			}()
		}
		// verify whether a server or a client sent the request
		if in.Metadata.BroadcastMsg.Sender == "client" {
			srv.broadcastSrv.addClientRequest(in.Metadata, ctx, finished)
			go srv.broadcastSrv.timeoutClientResponse(ctx, in, finished)
		} /*else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(nil), err))
		}*/
	}
}

func (srv *Server) RegisterBroadcastStruct(b broadcastStruct) {
	srv.broadcastSrv.b = b
}

func (srv *Server) RegisterMiddlewares(middlewares ...func()) {
	srv.broadcastSrv.middlewares = middlewares
}

func (srv *Server) RetToClient(resp responseTypes, err error, broadcastID string) {
	srv.broadcastSrv.Lock()
	defer srv.broadcastSrv.Unlock()
	if !srv.broadcastSrv.alreadyReturnedToClient(broadcastID, "") {
		srv.broadcastSrv.setReturnedToClient(broadcastID, true)
		srv.broadcastSrv.responseChan <- newResponseMessage(resp, err, broadcastID, clientResponse, srv.broadcastSrv.timeout)
	}
}

func (srv *Server) ListenForBroadcast() {
	go srv.broadcastSrv.run()
	go srv.broadcastSrv.handleClientResponses()
}

func RegisterBroadcastFunc[T requestTypes, V responseTypes](impl func(context.Context, T) (V, error)) func(context.Context, requestTypes) (responseTypes, error) {
	return func(ctx context.Context, req requestTypes) (resp responseTypes, err error) {
		return impl(ctx, req.(T))
	}
}

func (srv *Server) RegisterBroadcastFunc(method string) {
	srv.broadcastSrv.methods[method] = func(ctx context.Context, in requestTypes, broadcastID string) {
		cd := BroadcastCallData{
			Message:     in,
			Method:      method,
			BroadcastID: broadcastID,
		}
		srv.broadcastSrv.config.BroadcastCall(ctx, cd)
	}
}

func (srv *Server) RegisterConfig(c RawConfiguration) {
	srv.broadcastSrv.config = c
}
