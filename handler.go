package gorums

import (
	"context"
	"errors"
	"fmt"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var VERSION int = 2

func DefaultHandler[T RequestTypes, V ResponseTypes](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func BroadcastHandler[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	if VERSION == 1 {
		return BroadcastHandler1(impl, srv)
	}
	return BroadcastHandler2(impl, srv)
}

func BroadcastHandler1[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)

		srv.broadcastSrv.logger.Debug("received broadcast request", "req", req, "broadcastID", in.Metadata.BroadcastMsg.BroadcastID)

		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Debug("broadcast request not valid", "req", req, "err", err)
			return
		}
		data, err := srv.broadcastSrv.createRequest(ctx, in, finished)
		if err != nil {
			return
		}
		err = srv.broadcastSrv.processRequest(in, data)
		if err != nil {
			return
		}

		broadcastMetadata := newBroadcastMetadata(in.Metadata, 0)
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		impl(ctx, req, broadcaster)
	}
}

func BroadcastHandler2[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		defer ctx.Release()
		req := in.Message.(T)

		srv.broadcastSrv.logger.Debug("received broadcast request", "req", req, "broadcastID", in.Metadata.BroadcastMsg.BroadcastID)

		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Debug("broadcast request not valid", "req", req, "err", err)
			return
		}
		msg, err := srv.broadcastSrv.createRequest2(ctx, in, finished)
		if err != nil {
			return
		}
		err = srv.broadcastSrv.processRequest2(in, msg)
		if err != nil {
			return
		}

		broadcastMetadata := newBroadcastMetadata(in.Metadata, 0)
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		impl(ctx, req, broadcaster)
	}
}

func (srv *broadcastServer) createRequest2(ctx ServerCtx, in *Message, finished chan<- *Message) (content, error) {
	addOriginMethod(in.Metadata)
	return srv.state.createReq(ctx.Context, in.Metadata.BroadcastMsg.SenderType, in.Metadata.BroadcastMsg.OriginAddr, in.Metadata.BroadcastMsg.OriginMethod, in.Metadata.MessageID, in.Metadata.Method, finished)
}

func (srv *broadcastServer) processRequest2(in *Message, msg content) error {
	new, rC := srv.state.addOrUpdate2(in.Metadata.BroadcastMsg.BroadcastID)
	if new {
		go srv.handleReq(in.Metadata.BroadcastMsg.BroadcastID, rC, in.Metadata.BroadcastMsg.OriginAddr)
	}
	receiveChan := make(chan error)
	msg.receiveChan = receiveChan
	select {
	case rC.sendChan <- msg:
	case <-rC.ctx.Done():
		return errors.New("req is done")
	}
	return <-receiveChan
}

func (srv *broadcastServer) handleReq(broadcastID string, init *reqContent, originAddr string) {
	msg := content{
		ctx:        context.Background(),
		cancelFunc: init.cancelFunc,
		senderType: BroadcastServer,
		methods:    make([]string, 0),
	}
	go srv.router.createConnection(originAddr)
	defer msg.setDone()
	for {
		select {
		case <-init.ctx.Done():
			return
		case bMsg := <-init.broadcastChan:
			//slog.Info("received bMsg")
			if broadcastID != bMsg.broadcastID {
				//bMsg.receiveChan <- errors.New("wrong broadcastID")
				continue
			}
			if msg.isDone() {
				//bMsg.receiveChan <- errors.New("request is done and handled")
				continue
			}
			if bMsg.broadcast {
				if msg.hasBeenBroadcasted(bMsg.method) {
					//bMsg.receiveChan <- errors.New("already broadcasted")
					continue
				}
				err := srv.router.send(broadcastID, msg, bMsg.msg)
				if err != nil {
					//bMsg.receiveChan <- err
					continue
				}
				msg.addMethod(bMsg.method)
			} else {
				err := srv.router.send(broadcastID, msg, bMsg.reply)
				if err != nil {
					//slog.Warn("could not send", "sender", msg.senderType)
					_ = msg.setResponse(bMsg.reply)
					//if err == nil {
					//slog.Error("added response", "resp", bMsg.reply.response)
					//}
					//bMsg.receiveChan <- err
					continue
				}
				return
			}
		case new := <-init.sendChan:
			//slog.Info("received msg")
			if msg.isDone() {
				new.receiveChan <- errors.New("req is done")
				continue
			}
			msg.update(&new)
			if msg.canBeRouted() {
				//slog.Error("can be routed")
				err := srv.router.send(broadcastID, msg, msg.getResponse())
				if err != nil {
					//slog.Error("shoot", "err", err)
					new.receiveChan <- err
					continue
				}
				//slog.Error("was routed")
				return
			}
			new.receiveChan <- nil
		}
	}
}

func (srv *broadcastServer) createRequest(ctx ServerCtx, in *Message, finished chan<- *Message) (*content, error) {
	addOriginMethod(in.Metadata)
	data, err := srv.state.newData(ctx.Context, in.Metadata.BroadcastMsg.SenderType, in.Metadata.BroadcastMsg.OriginAddr, in.Metadata.BroadcastMsg.OriginMethod, in.Metadata.MessageID, in.Metadata.Method, finished)
	if err != nil {
		srv.logger.Debug("broadcast data could not be created", "data", data, "err", err)
		return nil, err
	}
	return data, nil
}

func (srv *broadcastServer) processRequest(in *Message, data *content) error {
	unlock, _, _ := srv.state.lockRequest(in.Metadata.BroadcastMsg.BroadcastID)
	defer unlock()

	err := srv.state.addOrUpdate(in.Metadata.BroadcastMsg.BroadcastID, data)
	if err != nil {
		srv.logger.Debug("broadcast request could not be added", "data", data, "err", err)
		//srv.broadcastSrv.router.unlock()
		return err
	}

	sent, err := srv.checkMsgAlreadyProcessed(in.Metadata.BroadcastMsg.BroadcastID)
	if sent {
		srv.logger.Debug("broadcast request already processed", "data", data, "err", err)
		//srv.broadcastSrv.router.unlock()
		return err
	}
	//srv.broadcastSrv.router.unlock()
	return nil
}

func (srv *broadcastServer) checkMsgAlreadyProcessed(broadcastID string) (bool, error) {
	data, err := srv.state.get(broadcastID)
	if err != nil {
		return true, err
	}
	if data.canBeRouted() {
		err = srv.router.send(broadcastID, data, data.getResponse())
		// should be removed regardless of a success.
		// it must have received a client request and
		// a client request.
		defer srv.state.remove(broadcastID)
		if err != nil {
			return true, err
		}
		// returning an error because request is already handled
		return true, nil
	}
	return false, nil
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

func (srv *Server) RegisterBroadcaster(b func(m BroadcastMetadata, o *BroadcastOrchestrator) Broadcaster) {
	srv.broadcastSrv.createBroadcaster = b
	srv.broadcastSrv.orchestrator = NewBroadcastOrchestrator(srv)
}

func (srv *broadcastServer) broadcastHandler(method string, req RequestTypes, broadcastID string, opts ...BroadcastOptions) {
	if VERSION == 1 {
		srv.broadcastHandler1(method, req, broadcastID, opts...)
	} else {
		srv.broadcastHandler2(method, req, broadcastID, opts...)
	}
}

func (srv *broadcastServer) sendToClientHandler(broadcastID string, resp ResponseTypes, err error) {
	if VERSION == 1 {
		srv.sendToClientHandler1(broadcastID, resp, err)
	} else {
		srv.sendToClientHandler2(broadcastID, resp, err)
	}
}

func (srv *broadcastServer) broadcastHandler2(method string, req RequestTypes, broadcastID string, opts ...BroadcastOptions) {
	rc, err := srv.state.getReqContent(broadcastID)
	if err != nil {
		return
	}
	select {
	case rc.broadcastChan <- bMsg{
		broadcast:   true,
		msg:         newBroadcastMessage2(broadcastID, req, method),
		method:      method,
		broadcastID: broadcastID,
	}:
	case <-rc.ctx.Done():
	}
}

func (srv *broadcastServer) sendToClientHandler2(broadcastID string, resp ResponseTypes, err error) {
	rc, err := srv.state.getReqContent(broadcastID)
	if err != nil {
		return
	}
	select {
	case rc.broadcastChan <- bMsg{
		reply: &reply{
			response: resp,
			err:      err,
		},
		broadcastID: broadcastID,
	}:
	case <-rc.ctx.Done():
	}
}

func (srv *broadcastServer) broadcastHandler1(method string, req RequestTypes, broadcastID string, opts ...BroadcastOptions) {
	options := BroadcastOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	finished := make(chan struct{})
	srv.broadcast(newBroadcastMessage(broadcastID, req, method, options, finished))

	// not broadcasting in a goroutine can lead to deadlock. All handlers are run sync
	// and thus the server have to return from the handler in order to process the next
	// request.
	//<-finished
	//}
}

func (srv *broadcastServer) sendToClientHandler1(broadcastID string, resp ResponseTypes, err error) {
	srv.sendToClient(newReply(resp, err, broadcastID))
}

func (srv *Server) SendToClientHandler(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.broadcastSrv.sendToClientHandler(broadcastID, resp, err)
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
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
		// drop request if a view change has occured
		srv.view.broadcastCall(ctx, cd)
		srv.viewMutex.RUnlock()
	})
}

func (srv *broadcastServer) registerSendToClientHandler(method string, handler clientHandler) {
	srv.router.addClientHandler(method, handler)
}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	// temporarily stop the broadcast server to prevent queueing
	// broadcast messages. Otherwise, beacuse the broadcast queueing
	// method holds a read lock and can thus prevent this method
	// from running.
	// handle all queued broadcast messages before changing the view
	srv.broadcastSrv.viewMutex.Lock()
	// delete all client requests. This resets all broadcast requests.
	srv.broadcastSrv.state.prune()
	srv.broadcastSrv.view = config
	srv.broadcastSrv.viewMutex.Unlock()
}
