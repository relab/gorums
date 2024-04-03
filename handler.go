package gorums

import (
	"context"
	"fmt"

	"github.com/relab/gorums/ordering"
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

		/*data, err := srv.broadcastSrv.storage.get(in.Metadata.BroadcastMsg.BroadcastID, false)
		if data.shouldWaitForClient() {
			srv.broadcastSrv.router.route(in.Metadata.BroadcastMsg.BroadcastID, data, data.getResponse())
			return
		}
		if err != nil {
			srv.broadcastSrv.storage.add(in.Metadata.BroadcastMsg.BroadcastID, data)
		} else {
			err := srv.broadcastSrv.storage.update(in.Metadata.BroadcastMsg.BroadcastID, data)
			if err != nil {
				return
			}
		}*/
		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Debug("broadcast request not valid", "req", req)
			return
		}
		if srv.broadcastSrv.inPending(ctx, in.Metadata, finished) {
			srv.broadcastSrv.logger.Debug("server has already processed the msg", "req", req)
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
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
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
	if in.Metadata.BroadcastMsg.SenderType == "" {
		return fmt.Errorf("senderType cannot be empty. got: %v", in.Metadata.BroadcastMsg.SenderType)
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
	//if !srv.alreadyBroadcasted(broadcastID, method) || options.OmitUniquenessChecks {
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
	//<-finished
	//}
}

func (srv *Server) RegisterBroadcaster(b func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster) {
	srv.broadcastSrv.createBroadcaster = b
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
}

func (srv *Server) RetToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.broadcastSrv.sendToClient(broadcastID, resp, err)
}

func (srv *broadcastServer) registerReturnToClientHandler(method string, handler clientHandler) {
	srv.clientHandlers[method] = handler
	srv.router.addClientHandler(method, handler)
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.handlers[method] = func(ctx context.Context, in RequestTypes, broadcastID, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string) {
		cd := broadcastCallData{
			Message:         in,
			Method:          method,
			BroadcastID:     broadcastID,
			SenderType:      BroadcastServer,
			SenderID:        id,
			SenderAddr:      addr,
			OriginAddr:      originAddr,
			OriginMethod:    originMethod,
			ServerAddresses: options.ServerAddresses,
		}
		srv.viewMutex.RLock()
		srv.view.broadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	}
	srv.router.addServerHandler(method, func(ctx context.Context, in RequestTypes, broadcastID, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string) {
		cd := broadcastCallData{
			Message:         in,
			Method:          method,
			BroadcastID:     broadcastID,
			SenderType:      BroadcastServer,
			SenderID:        id,
			SenderAddr:      addr,
			OriginAddr:      originAddr,
			OriginMethod:    originMethod,
			ServerAddresses: options.ServerAddresses,
		}
		srv.viewMutex.RLock()
		srv.view.broadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	})
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
