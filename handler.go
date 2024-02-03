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
		srv.broadcastSrv.Lock()
		defer srv.broadcastSrv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		// the client can specify middleware, e.g. authentication, to return early.
		err := srv.broadcastSrv.runMiddleware()
		if err != nil {
			// return if any of the middlewares return an error
			return
		}
		srv.broadcastSrv.b.reset(in.Metadata.BroadcastMsg.BroadcastID)
		_ = impl(ctx, req, srv.broadcastSrv.b.(V))
		srv.broadcastSrv.determineBroadcast(ctx, in.Metadata.BroadcastMsg.GetBroadcastID(), srv.getOwnAddr())
		// verify whether a server or a client sent the request
		if in.Metadata.BroadcastMsg.Sender == "client" {
			srv.broadcastSrv.addClientRequest(in.Metadata, ctx, finished)
			go srv.broadcastSrv.timeoutClientResponse(ctx, in, finished)
		} /*else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(nil), err))
		}*/
		srv.broadcastSrv.determineReturnToClient(ctx, in.Metadata.BroadcastMsg.GetBroadcastID())
	}
}

func (srv *Server) getOwnAddr() string {
	return ""
}

func (srv *broadcastServer) determineReturnToClient(ctx ServerCtx, broadcastID string) {
	if srv.b.shouldReturnToClient() {
		for i, resp := range srv.b.getResponses() {
			if !srv.alreadyReturnedToClient(broadcastID) {
				srv.setReturnedToClient(broadcastID, true)
				go func(i int, resp responseTypes) {
					srv.responseChan <- newResponseMessage(resp, srv.b.getError(i), broadcastID, clientResponse, srv.timeout)
				}(i, resp)
			}
		}
	}
}

func (srv *broadcastServer) determineBroadcast(ctx ServerCtx, broadcastID, from string) {
	if srv.b.shouldBroadcast() {
		for i, method := range srv.b.getMethods() {
			// maybe let this be an option for the implementer?
			if !srv.alreadyBroadcasted(broadcastID, method) {
				// how to define individual request message to each node?
				//	- maybe create one request for each node and send a list of requests?
				go srv.broadcast(newBroadcastMessage(ctx, srv.b.getRequest(i), method, broadcastID, from))
			}
		}
	}
}

func (srv *Server) RegisterBroadcastStruct(b broadcastStruct) {
	srv.broadcastSrv.b = b
}

func (srv *Server) RegisterMiddlewares(middlewares ...func() error) {
	srv.broadcastSrv.middlewares = middlewares
}

func (srv *Server) RetToClient(resp responseTypes, err error, broadcastID string) {
	srv.broadcastSrv.Lock()
	defer srv.broadcastSrv.Unlock()
	if !srv.broadcastSrv.alreadyReturnedToClient(broadcastID) {
		srv.broadcastSrv.setReturnedToClient(broadcastID, true)
		srv.broadcastSrv.responseChan <- newResponseMessage(resp, err, broadcastID, clientResponse, srv.broadcastSrv.timeout)
	}
}

func (srv *Server) ListenForBroadcast() {
	go srv.broadcastSrv.run()
	go srv.broadcastSrv.handleClientResponses()
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.methods[method] = func(ctx context.Context, in requestTypes, broadcastID string) {
		cd := BroadcastCallData{
			Message:     in,
			Method:      method,
			BroadcastID: broadcastID,
		}
		srv.config.BroadcastCall(ctx, cd)
	}
}

func (srv *Server) RegisterConfig(c RawConfiguration) {
	srv.broadcastSrv.config = c
}
