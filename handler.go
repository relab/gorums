package gorums

import (
	"context"
	"fmt"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
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

func BroadcastHandler[T RequestTypes, V broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)
		srv.broadcastSrv.logger.Debug("received broadcast request", "req", req)
		addOriginMethod(in.Metadata)
		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Info("broadcast request not valid", "req", req)
			return
		}
		// add the request as a client request
		srv.broadcastSrv.addClientRequest(in.Metadata, ctx, finished)
		broadcastMetadata := newBroadcastMetadata(in.Metadata)
		srv.broadcastSrv.broadcaster.setMetadata(broadcastMetadata)
		impl(ctx, req, srv.broadcastSrv.broadcaster.(V))
		srv.broadcastSrv.broadcaster.resetMetadata()
	}
}

func addOriginMethod(md *ordering.Metadata) {
	if md.BroadcastMsg.SenderType != BroadcastClient {
		return
	}
	// keep track of the method called by the user
	md.BroadcastMsg.OriginMethod = md.Method
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

func (srv *broadcastServer) broadcasterHandler(method string, req RequestTypes, metadata BroadcastMetadata, opts ...BroadcastOptions) {
	options := BroadcastOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	if !srv.alreadyBroadcasted(metadata.BroadcastID, method) || options.OmitUniquenessChecks {
		// it is possible to broadcast outside a server handler and it is thus necessary
		// to check whether the provided broadcastID is valid. It will also add the
		// necessary fields to correctly route the request.
		valid := srv.updateMetadata(&metadata)
		if !valid {
			return
		}
		finished := make(chan struct{})
		srv.broadcastChan <- newBroadcastMessage(metadata, req, method, metadata.BroadcastID, options.ServerAddresses, finished)

		// not broadcasting in a goroutine can lead to deadlock. All handlers are run sync
		// and thus the server have to return from the handler in order to process the next
		// request.
		<-finished
	}
}

func (srv *broadcastServer) updateMetadata(metadata *BroadcastMetadata) bool {
	clientReq, ok := srv.clientReqs.Get(metadata.BroadcastID)
	if !ok {
		return false
	}
	metadata.OriginAddr = clientReq.metadata.BroadcastMsg.OriginAddr
	metadata.OriginMethod = clientReq.metadata.BroadcastMsg.OriginMethod
	return true
}

func (srv *Server) RegisterBroadcaster(b broadcaster, configureHandlers func(brh BroadcastHandlerFunc, ch BroadcastSendToClientHandlerFunc), configureMetadata func(metadata BroadcastMetadata), resetMetadata func()) {
	srv.broadcastSrv.broadcaster = b
	srv.broadcastSrv.broadcaster.setMetadataHandler(configureMetadata, resetMetadata)
	configureHandlers(srv.broadcastSrv.broadcasterHandler, srv.broadcastSrv.sendToClient)
}

func (srv *Server) RetToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.broadcastSrv.sendToClient(broadcastID, resp, err)
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
			SenderType:      BroadcastServer,
			SenderAddr:      srv.addr,
			OriginAddr:      md.OriginAddr,
			OriginMethod:    md.OriginMethod,
			ServerAddresses: srvAddrs,
		}
		srv.view.broadcastCall(ctx, cd)
	}
}

//func (srv *Server) View() serverView {
//	return srv.broadcastSrv.view
//}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	srvAddrs := make([]string, 0, len(config))
	for _, node := range config.Nodes() {
		srvAddrs = append(srvAddrs, node.Address())
	}
	srv.broadcastSrv.peers = srvAddrs
	//srvAddrs := make([]string, len(srv.broadcastSrv.peers))
	//copy(srvAddrs, srv.broadcastSrv.peers)
	//ownAddrIncluded := false
	//for _, addr := range srvAddrs {
	//	if addr == srv.broadcastSrv.addr {
	//		ownAddrIncluded = true
	//		break
	//	}
	//}
	//if !ownAddrIncluded {
	//	srvAddrs = append(srvAddrs, srv.broadcastSrv.addr)
	//}
	//config, err := NewRawConfiguration(srv.broadcastSrv.mgr, WithNodeListBroadcast(srvAddrs))
	srv.broadcastSrv.view = config
	//srv.broadcastSrv.timeout = mgr.opts.nodeDialTimeout
	srv.broadcastSrv.timeout = 5 * time.Second
}

//func (srv *Server) RegisterView(listenAddr string, srvAddrs []string, opts ...ManagerOption) error {
//	if len(opts) <= 0 {
//		opts = make([]ManagerOption, 2)
//		opts[0] = WithDialTimeout(50 * time.Millisecond)
//		opts[1] = WithGrpcDialOptions(
//			grpc.WithBlock(),
//			grpc.WithTransportCredentials(insecure.NewCredentials()),
//		)
//	}
//	srv.broadcastSrv.mgr = NewRawManager(opts...)
//	srv.broadcastSrv.peers = srvAddrs
//	srv.broadcastSrv.addr = listenAddr
//	if srv.broadcastSrv.addr == "" {
//		slog.Debug("listenAddr cannot be empty")
//		return fmt.Errorf("listenAddr cannot be empty")
//	}
//	return srv.configureView()
//}
//
//func (srv *Server) configureView() error {
//	if srv.broadcastSrv.mgr == nil {
//		slog.Debug("manager not created yet")
//		return fmt.Errorf("manager not created yet")
//	}
//	srvAddrs := make([]string, len(srv.broadcastSrv.peers))
//	copy(srvAddrs, srv.broadcastSrv.peers)
//	ownAddrIncluded := false
//	for _, addr := range srvAddrs {
//		if addr == srv.broadcastSrv.addr {
//			ownAddrIncluded = true
//			break
//		}
//	}
//	if !ownAddrIncluded {
//		srvAddrs = append(srvAddrs, srv.broadcastSrv.addr)
//	}
//	config, err := NewRawConfiguration(srv.broadcastSrv.mgr, WithNodeListBroadcast(srvAddrs))
//	srv.broadcastSrv.view = config
//	//srv.broadcastSrv.timeout = mgr.opts.nodeDialTimeout
//	srv.broadcastSrv.timeout = 5 * time.Second
//	return err
//}
