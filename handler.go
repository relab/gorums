package gorums

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func DefaultHandler[T RequestTypes, V ResponseTypes](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func BroadcastHandler[T RequestTypes, V iBroadcastStruct](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.broadcastSrv.Lock()
		defer srv.broadcastSrv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		doneChan := make(chan struct{})
		go func() {
			defer func() { close(doneChan) }()
			// guard:
			// - A broadcastID should be non-empty:
			// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
			if err := srv.broadcastSrv.validateMessage(in); err != nil {
				return
			}
			addOriginMethod(in.Metadata)
			broadcastMetadata := newBroadcastMetadata(in.Metadata)
			// the client can specify middleware, e.g. authentication, to return early.
			if err := srv.broadcastSrv.runMiddleware(broadcastMetadata); err != nil {
				// return if any of the middlewares return an error
				return
			}
			srv.broadcastSrv.bNew.setMetadata(broadcastMetadata)
			// add the request as a client request
			srv.broadcastSrv.addClientRequest(in.Metadata, ctx, finished)
			impl(ctx, req, srv.broadcastSrv.bNew.(V))
			//// verify whether a server or a client sent the request
			//if in.Metadata.BroadcastMsg.Sender == BroadcastClient {
			//	go srv.broadcastSrv.timeoutClientResponse(ctx, in, finished)
			//}
		}()
		<-doneChan
	}
}

func addOriginMethod(md *ordering.Metadata) {
	if md.BroadcastMsg.Sender != BroadcastClient {
		return
	}
	md.BroadcastMsg.OriginMethod = md.Method
	//tmp := strings.Split(md.Method, ".")
	//m := ""
	//if len(tmp) >= 1 {
	//	m = tmp[len(tmp)-1]
	//}
	//md.BroadcastMsg.OriginMethod = "Client" + m
}

func (srv *broadcastServer) validateMessage(in *Message) error {
	if in == nil {
		return fmt.Errorf("message cannot be empty. got: %v", in)
	}
	if in.Metadata == nil {
		return fmt.Errorf("metadata cannot be empty. got: %v", in.Metadata)
	}
	if in.Metadata.BroadcastMsg == nil {
		return fmt.Errorf("broadcastMsg cannot be empty. got: %v", in.Metadata.BroadcastMsg)
	}
	if in.Metadata.BroadcastMsg.BroadcastID == "" {
		return fmt.Errorf("broadcastID cannot be empty. got: %v", in.Metadata.BroadcastMsg.BroadcastID)
	}
	// check and update TTL
	// check deadline
	return nil
}

func (srv *broadcastServer) broadcastStructHandler(method string, req RequestTypes, metadata BroadcastMetadata, opts ...BroadcastOptions) {
	options := BroadcastOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	// maybe let this be an option for the implementer?
	if !srv.alreadyBroadcasted(metadata.BroadcastID, method) || options.OmitUniquenessChecks {
		finished := make(chan struct{})

		// how to define individual request message to each node?
		//	- maybe create one request for each node and send a list of requests?
		srv.broadcastChan <- newBroadcastMessage(metadata, req, method, metadata.BroadcastID, options.ServerAddresses, finished)
		<-finished
	}
}

func (srv *Server) RegisterBroadcastStruct(b iBroadcastStruct, configureHandlers func(bh BroadcastHandlerFunc, ch BroadcastReturnToClientHandlerFunc), configureMetadata func(metadata BroadcastMetadata)) {
	srv.broadcastSrv.bNew = b
	srv.broadcastSrv.bNew.setMetadataHandler(configureMetadata)
	configureHandlers(srv.broadcastSrv.broadcastStructHandler, srv.broadcastSrv.clientReturn)
}

func (srv *Server) RegisterMiddlewares(middlewares ...func(BroadcastMetadata) error) {
	srv.broadcastSrv.middlewares = middlewares
}

func (srv *Server) RetToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.broadcastSrv.returnToClient(broadcastID, resp, err)
}

func (srv *Server) ListenForBroadcast() {
	go srv.broadcastSrv.run()
	go srv.broadcastSrv.handleClientResponses()
}

func (srv *broadcastServer) registerReturnToClientHandler(method string, handler func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)) {
	srv.clientHandlers[method] = handler
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.handlers[method] = func(ctx context.Context, in RequestTypes, md BroadcastMetadata, srvAddrs []string) {
		cd := broadcastCallData{
			Message:         in,
			Method:          method,
			BroadcastID:     md.BroadcastID,
			Sender:          BroadcastServer,
			SenderAddr:      srv.addr,
			SenderID:        srv.id,
			OriginID:        md.OriginID,
			OriginAddr:      md.OriginAddr,
			OriginMethod:    md.OriginMethod,
			ServerAddresses: srvAddrs,
		}
		srv.view.broadcastCall(ctx, cd)
	}
}

func (srv *Server) RegisterView(ownAddr string, srvAddrs []string, opts ...ManagerOption) error {
	if len(opts) <= 0 {
		opts = make([]ManagerOption, 2)
		opts[0] = WithDialTimeout(50 * time.Millisecond)
		opts[1] = WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}
	srv.broadcastSrv.addr = ownAddr
	mgr := NewRawManager(opts...)
	config, err := NewRawConfiguration(mgr, WithNodeListBroadcast(srvAddrs))
	srv.broadcastSrv.view = config
	//srv.broadcastSrv.timeout = mgr.opts.nodeDialTimeout
	srv.broadcastSrv.timeout = 5 * time.Second
	return err
}
