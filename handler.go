package gorums

import (
	"context"
	"fmt"

	"github.com/relab/gorums/broadcast"
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

func ClientHandler[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage](impl clientImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, _ chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)
		impl(ctx, req, in.Metadata.BroadcastMsg.BroadcastID)
	}
}

func BroadcastHandler[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// release immediately to process next message
		ctx.Release()

		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			if srv.broadcastSrv.logger != nil {
				srv.broadcastSrv.logger.Debug("broadcast request not valid", "metadata", in.Metadata, "err", err)
			}
			return
		}

		// interface conversion can fail if proto message of the wrong type is given.
		// this happens when Cancellations arrive because the proto message is nil but
		// we still want to process the message.
		req, ok := in.Message.(T)
		if !ok && in.Metadata.Method != Cancellation {
			return
		}
		if in.Metadata.BroadcastMsg.IsBroadcastClient {
			// keep track of the method called by the user
			in.Metadata.BroadcastMsg.OriginMethod = in.Metadata.Method
		}
		broadcastMetadata := newBroadcastMetadata(in.Metadata)
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		// due to ordering we wrap the actual implementation function to be able to
		// run it at a later time.
		run := func(reqCtx context.Context) {
			// we need to pass in the reqCtx because we can only retrieve
			// it after we have gotten a response from the shard. The reqCtx
			// is used for cancellations.
			ctx.Context = reqCtx
			impl(ctx, req, broadcaster)
		}

		msg := broadcast.Content{}
		createRequest(&msg, ctx, in, finished, run)

		// we are not interested in the server context as this is tied to the previous hop.
		// instead we want to check whether the client has cancelled the broadcast request
		// and if so, we return a cancelled context. This enables the implementer to listen
		// for cancels and do proper actions.
		err, reqCtx := srv.broadcastSrv.manager.Process(msg)
		if err != nil {
			return
		}

		run(reqCtx)
	}
}

func createRequest(msg *broadcast.Content, ctx ServerCtx, in *Message, finished chan<- *Message, run func(context.Context)) {
	msg.BroadcastID = in.Metadata.BroadcastMsg.BroadcastID
	msg.IsBroadcastClient = in.Metadata.BroadcastMsg.IsBroadcastClient
	msg.OriginAddr = in.Metadata.BroadcastMsg.OriginAddr
	msg.OriginMethod = in.Metadata.BroadcastMsg.OriginMethod
	msg.CurrentMethod = in.Metadata.Method
	msg.Ctx = ctx.Context
	msg.Run = run
	if msg.OriginAddr == "" && msg.IsBroadcastClient {
		msg.SendFn = createSendFn(in.Metadata.MessageID, in.Metadata.Method, finished, ctx)
	}
	if in.Metadata.Method == Cancellation {
		msg.IsCancellation = true
	}
}

func createSendFn(msgID uint64, method string, finished chan<- *Message, ctx ServerCtx) func(resp protoreflect.ProtoMessage, err error) {
	return func(resp protoreflect.ProtoMessage, err error) {
		md := &ordering.Metadata{
			MessageID: msgID,
			Method:    method,
		}
		msg := WrapMessage(md, resp, err)
		SendMessage(ctx, finished, msg)
	}
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
	if in.Metadata.BroadcastMsg.BroadcastID <= 0 {
		return fmt.Errorf("broadcastID cannot be empty. got: %v", in.Metadata.BroadcastMsg.BroadcastID)
	}
	// check and update TTL
	// check deadline
	return nil
}

func (srv *Server) RegisterBroadcaster(b func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster) {
	srv.broadcastSrv.createBroadcaster = b
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
}

func (srv *broadcastServer) broadcastHandler(method string, req protoreflect.ProtoMessage, broadcastID uint64, opts ...broadcast.BroadcastOptions) {
	//srv.state.ProcessBroadcast(broadcastID, req, method)
	srv.manager.Broadcast(broadcastID, req, method, opts...)
}

func (srv *broadcastServer) sendToClientHandler(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	//srv.state.ProcessSendToClient(broadcastID, resp, err)
	srv.manager.SendToClient(broadcastID, resp, err)
}

func (srv *broadcastServer) forwardHandler(req RequestTypes, method string, broadcastID uint64, forwardAddr, originAddr string) {
	cd := BroadcastCallData{
		Message:           req,
		Method:            method,
		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        originAddr,
		ServerAddresses:   []string{forwardAddr},
	}
	srv.viewMutex.RLock()
	// drop request if a view change has occured
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *broadcastServer) cancelHandler(broadcastID uint64, srvAddrs []string) {
	srv.manager.Cancel(broadcastID, srvAddrs)
}

func (srv *broadcastServer) doneHandler(broadcastID uint64) {
	srv.manager.Done(broadcastID)
}

func (srv *broadcastServer) canceler(broadcastID uint64, srvAddrs []string) {
	cd := BroadcastCallData{
		Message:         nil,
		Method:          Cancellation,
		BroadcastID:     broadcastID,
		ServerAddresses: srvAddrs,
	}
	srv.viewMutex.RLock()
	// drop request if a view change has occured
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *broadcastServer) serverBroadcastHandler(method string, req RequestTypes, opts ...broadcast.BroadcastOptions) {
	cd := BroadcastCallData{
		Message:           req,
		Method:            method,
		BroadcastID:       srv.manager.NewBroadcastID(),
		OriginAddr:        "server",
		IsBroadcastClient: false,
	}
	srv.viewMutex.RLock()
	// drop request if a view change has occured
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *Server) SendToClientHandler(resp protoreflect.ProtoMessage, err error, broadcastID uint64) {
	srv.broadcastSrv.sendToClientHandler(broadcastID, resp, err)
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.manager.AddHandler(method, broadcast.ServerHandler(func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options broadcast.BroadcastOptions, id uint32, addr string) {
		cd := BroadcastCallData{
			Message:           in,
			Method:            method,
			BroadcastID:       broadcastID,
			IsBroadcastClient: false,
			SenderAddr:        addr,
			OriginAddr:        originAddr,
			OriginMethod:      originMethod,
			ServerAddresses:   options.ServerAddresses,
		}
		srv.viewMutex.RLock()
		// drop request if a view change has occured
		srv.view.BroadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	}))
	/*srv.manager.AddServerHandler(method, func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options broadcast.BroadcastOptions, id uint32, addr string) {
		cd := broadcastCallData{
			Message:           in,
			Method:            method,
			BroadcastID:       broadcastID,
			IsBroadcastClient: false,
			SenderAddr:        addr,
			OriginAddr:        originAddr,
			OriginMethod:      originMethod,
			ServerAddresses:   options.ServerAddresses,
		}
		srv.viewMutex.RLock()
		// drop request if a view change has occured
		srv.view.broadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	})*/
}

func (srv *broadcastServer) registerSendToClientHandler(method string) {
	//srv.manager.AddClientHandler(method)
	srv.manager.AddHandler(method, nil)
}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	// temporarily stop the broadcast server to prevent queueing
	// broadcast messages. Otherwise, beacuse the broadcast queueing
	// method holds a read lock and can thus prevent this method
	// from running.
	// handle all queued broadcast messages before changing the view
	srv.broadcastSrv.viewMutex.Lock()
	// delete all client requests. This resets all broadcast requests.
	srv.broadcastSrv.manager.ResetState()
	srv.broadcastSrv.view = config
	srv.broadcastSrv.viewMutex.Unlock()
}
