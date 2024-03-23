package gorums

import (
	"context"
	"fmt"

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

func BroadcastHandler[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)
		srv.broadcastSrv.logger.Debug("received broadcast request", "req", req)
		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Info("broadcast request not valid", "req", req)
			return
		}
		addOriginMethod(in.Metadata)
		// add the request as a client request
		count, err := srv.broadcastSrv.addClientRequest(in.Metadata, ctx, finished)
		if err != nil {
			// request has been handled and is no longer valid
			return
		}
		broadcastMetadata := newBroadcastMetadata(in.Metadata, count)
		broadcaster := srv.broadcastSrv.broadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		impl(ctx, req, broadcaster)
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

func (srv *broadcastServer) broadcasterHandler(method string, req RequestTypes, broadcastID string, opts ...BroadcastOptions) {
	options := BroadcastOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	if !srv.alreadyBroadcasted(broadcastID, method) || options.OmitUniquenessChecks {
		// it is possible to broadcast outside a server handler and it is thus necessary
		// to check whether the provided broadcastID is valid. It will also add the
		// necessary fields to correctly route the request.
		if handled := srv.clientReqs.IsHandled(broadcastID); handled {
			return
		}
		finished := make(chan struct{})
		srv.broadcastChan <- newBroadcastMessage(broadcastID, req, method, options, finished)

		// not broadcasting in a goroutine can lead to deadlock. All handlers are run sync
		// and thus the server have to return from the handler in order to process the next
		// request.
		<-finished
	}
}

func (srv *Server) RegisterBroadcaster(b func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster) {
	srv.broadcastSrv.broadcaster = b
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
}

func (srv *Server) RetToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.broadcastSrv.sendToClient(broadcastID, resp, err)
}

func (srv *broadcastServer) registerReturnToClientHandler(method string, handler func(addr, broadcastID string, req protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error)) {
	srv.clientHandlers[method] = handler
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.handlers[method] = func(ctx context.Context, in RequestTypes, req clientRequest, options BroadcastOptions) {
		srv.propertiesMutex.Lock()
		cd := broadcastCallData{
			Message:         in,
			Method:          method,
			BroadcastID:     req.metadata.BroadcastMsg.BroadcastID,
			SenderType:      BroadcastServer,
			SenderID:        srv.id,
			SenderAddr:      srv.addr,
			OriginAddr:      req.metadata.BroadcastMsg.OriginAddr,
			OriginMethod:    req.metadata.BroadcastMsg.OriginMethod,
			ServerAddresses: options.ServerAddresses,
		}
		srv.propertiesMutex.Unlock()
		srv.view.broadcastCall(ctx, cd)
	}
}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	// temporarily stop the broadcast server to prevent queueing
	// broadcast messages. Otherwise, beacuse the broadcast queueing
	// method holds a read lock and can thus prevent this method
	// from running.
	srv.broadcastSrv.stop()
	// handle all queued broadcast messages before changing the view
	srv.broadcastSrv.viewMutex.Lock()
	// delete all client requests. This resets all broadcast requests.
	srv.broadcastSrv.clientReqs.Reset()
	srv.broadcastSrv.view = config
	srv.broadcastSrv.viewMutex.Unlock()
	// restart the server to resume progress
	srv.broadcastSrv.start()
}
