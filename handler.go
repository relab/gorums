package gorums

import (
	"context"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func DefaultHandler[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		_ = SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func ClientHandler[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage](impl clientImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, _ chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)
		// err := status.FromProto(in.Metadata.GetStatus()).Err()
		_, _ = impl(ctx, req, in.Metadata.GetBroadcastMsg().GetBroadcastID())
	}
}

func BroadcastHandler[T protoreflect.ProtoMessage, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// release immediately to process next message
		ctx.Release()

		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := in.isValid(); err != nil {
			if srv.broadcastSrv.logger != nil {
				srv.broadcastSrv.logger.Debug("invalid broadcast request", "metadata", in.Metadata, "err", err)
			}
			return
		}

		// interface conversion can fail if proto message of the wrong type is given.
		// this happens when Cancellations arrive because the proto message is nil but
		// we still want to process the message.
		req, ok := in.Message.(T)
		if !ok && in.Metadata.GetMethod() != Cancellation {
			return
		}
		if in.Metadata.GetBroadcastMsg().GetIsBroadcastClient() {
			// keep track of the method called by the user
			in.Metadata.GetBroadcastMsg().SetOriginMethod(in.Metadata.GetMethod())
		}
		// due to ordering we wrap the actual implementation function to be able to
		// run it at a later time.
		run := func(reqCtx context.Context, enqueueBroadcast func(*broadcast.Msg) error) {
			// we need to pass in the reqCtx and broadcastChan because we can only retrieve
			// them after we have gotten a response from the shard. The reqCtx
			// is used for cancellations and broadcastChan for broadcasts.
			broadcastMetadata := newBroadcastMetadata(in.Metadata)
			broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator, enqueueBroadcast).(V)
			ctx.Context = reqCtx
			impl(ctx, req, broadcaster)
		}

		msg := broadcast.Content{}
		createRequest(&msg, ctx, in, finished, run)

		// we are not interested in the server context as this is tied to the previous hop.
		// instead we want to check whether the client has cancelled the broadcast request
		// and if so, we return a cancelled context. This enables the implementer to listen
		// for cancels and do proper actions.
		reqCtx, enqueueBroadcast, err := srv.broadcastSrv.manager.Process(&msg)
		if err != nil {
			return
		}

		run(reqCtx, enqueueBroadcast)
	}
}

func createRequest(msg *broadcast.Content, ctx ServerCtx, in *Message, finished chan<- *Message, run func(context.Context, func(*broadcast.Msg) error)) {
	msg.BroadcastID = in.Metadata.GetBroadcastMsg().GetBroadcastID()
	msg.IsBroadcastClient = in.Metadata.GetBroadcastMsg().GetIsBroadcastClient()
	msg.OriginAddr = in.Metadata.GetBroadcastMsg().GetOriginAddr()
	msg.OriginMethod = in.Metadata.GetBroadcastMsg().GetOriginMethod()
	msg.SenderAddr = in.Metadata.GetBroadcastMsg().GetSenderAddr()
	if msg.SenderAddr == "" && msg.IsBroadcastClient {
		msg.SenderAddr = "client"
	}
	if in.Metadata.GetBroadcastMsg().GetOriginDigest() != nil {
		msg.OriginDigest = in.Metadata.GetBroadcastMsg().GetOriginDigest()
	}
	if in.Metadata.GetBroadcastMsg().GetOriginSignature() != nil {
		msg.OriginSignature = in.Metadata.GetBroadcastMsg().GetOriginSignature()
	}
	if in.Metadata.GetBroadcastMsg().GetOriginPubKey() != "" {
		msg.OriginPubKey = in.Metadata.GetBroadcastMsg().GetOriginPubKey()
	}
	msg.CurrentMethod = in.Metadata.GetMethod()
	msg.Ctx = ctx.Context
	msg.Run = run
	if msg.OriginAddr == "" && msg.IsBroadcastClient {
		msg.SendFn = createSendFn(in.Metadata.GetMessageID(), in.Metadata.GetMethod(), finished, ctx)
	}
	if in.Metadata.GetMethod() == Cancellation {
		msg.IsCancellation = true
	}
}

func createSendFn(msgID uint64, method string, finished chan<- *Message, ctx ServerCtx) func(resp protoreflect.ProtoMessage, err error) error {
	return func(resp protoreflect.ProtoMessage, err error) error {
		md := ordering.NewGorumsMetadata(ctx, msgID, method)
		msg := WrapMessage(md, resp, err)
		return SendMessage(ctx, finished, msg)
	}
}

func (srv *Server) RegisterBroadcaster(broadcaster func(m BroadcastMetadata, o *BroadcastOrchestrator, e EnqueueBroadcast) Broadcaster) {
	srv.broadcastSrv.createBroadcaster = broadcaster
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
}

func (srv *broadcastServer) broadcastHandler(method string, req protoreflect.ProtoMessage, broadcastID uint64, enqueueBroadcast EnqueueBroadcast, opts ...broadcast.BroadcastOptions) error {
	return srv.manager.Broadcast(broadcastID, req, method, enqueueBroadcast, opts...)
}

func (srv *broadcastServer) sendToClientHandler(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueBroadcast EnqueueBroadcast) error {
	return srv.manager.SendToClient(broadcastID, resp, err, enqueueBroadcast)
}

func (srv *broadcastServer) forwardHandler(req protoreflect.ProtoMessage, method string, broadcastID uint64, forwardAddr, originAddr string) {
	cd := BroadcastCallData{
		Message:           req,
		Method:            method,
		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        originAddr,
		ServerAddresses:   []string{forwardAddr},
	}
	srv.viewMutex.RLock()
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *broadcastServer) cancelHandler(broadcastID uint64, srvAddrs []string, enqueueBroadcast EnqueueBroadcast) error {
	return srv.manager.Cancel(broadcastID, srvAddrs, enqueueBroadcast)
}

func (srv *broadcastServer) doneHandler(broadcastID uint64, enqueueBroadcast EnqueueBroadcast) {
	srv.manager.Done(broadcastID, enqueueBroadcast)
}

func (srv *broadcastServer) canceler(broadcastID uint64, srvAddrs []string) {
	cd := BroadcastCallData{
		Message:         nil,
		Method:          Cancellation,
		BroadcastID:     broadcastID,
		ServerAddresses: srvAddrs,
	}
	srv.viewMutex.RLock()
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *broadcastServer) serverBroadcastHandler(method string, req protoreflect.ProtoMessage, opts ...broadcast.BroadcastOptions) {
	cd := BroadcastCallData{
		Message:           req,
		Method:            method,
		BroadcastID:       srv.manager.NewBroadcastID(),
		OriginAddr:        broadcast.ServerOriginAddr,
		IsBroadcastClient: false,
	}
	srv.viewMutex.RLock()
	srv.view.BroadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()
}

func (srv *Server) SendToClientHandler(resp protoreflect.ProtoMessage, err error, broadcastID uint64, enqueueBroadcast EnqueueBroadcast) error {
	return srv.broadcastSrv.sendToClientHandler(broadcastID, resp, err, enqueueBroadcast)
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.manager.AddHandler(method, broadcast.ServerHandler(func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options broadcast.BroadcastOptions, id uint32, addr string, originDigest, originSignature []byte, originPubKey string) {
		cd := BroadcastCallData{
			Message:           in,
			Method:            method,
			BroadcastID:       broadcastID,
			IsBroadcastClient: false,
			SenderAddr:        addr,
			OriginAddr:        originAddr,
			OriginMethod:      originMethod,
			ServerAddresses:   options.ServerAddresses,
			SkipSelf:          options.SkipSelf,
			OriginDigest:      originDigest,
			OriginSignature:   originSignature,
			OriginPubKey:      originPubKey,
		}
		srv.viewMutex.RLock()
		srv.view.BroadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	}))
}

func (srv *broadcastServer) registerSendToClientHandler(method string) {
	srv.manager.AddHandler(method, nil)
}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	srv.broadcastSrv.viewMutex.Lock()
	srv.broadcastSrv.manager.ResetState()
	srv.broadcastSrv.view = config
	srv.broadcastSrv.viewMutex.Unlock()
}
