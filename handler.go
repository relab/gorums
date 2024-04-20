package gorums

import (
	"context"
	"errors"
	"fmt"

	"github.com/relab/gorums/broadcast"
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

		broadcastMetadata := newBroadcastMetadata(in.Metadata)
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		impl(ctx, req, broadcaster)
	}
}

func BroadcastHandler2[T RequestTypes, V Broadcaster](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		ctx.Release()
		req := in.Message.(T)

		//srv.broadcastSrv.logger.Debug("received broadcast request", "req", req, "broadcastID", in.Metadata.BroadcastMsg.BroadcastID)

		// guard:
		// - A broadcastID should be non-empty:
		// - Maybe the request should be unique? Remove duplicates of the same broadcast? <- Most likely no (up to the implementer)
		if err := srv.broadcastSrv.validateMessage(in); err != nil {
			srv.broadcastSrv.logger.Debug("broadcast request not valid", "req", req, "err", err)
			return
		}
		msg := content2{}
		createRequest2(&msg, ctx, in, finished)
		//slog.Info("got req", "broadcastID", msg.broadcastID, "req", req, "method", in.Metadata.Method)

		err := srv.broadcastSrv.processRequest2(msg)
		if err != nil {
			//slog.Error("broadcast handler", "err", err)
			return
		}

		broadcastMetadata := newBroadcastMetadata(in.Metadata)
		broadcaster := srv.broadcastSrv.createBroadcaster(broadcastMetadata, srv.broadcastSrv.orchestrator).(V)
		impl(ctx, req, broadcaster)
	}
}

func alreadyBroadcasted(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func createRequest2(msg *content2, ctx ServerCtx, in *Message, finished chan<- *Message) {
	addOriginMethod(in.Metadata)
	msg.broadcastID = in.Metadata.BroadcastMsg.BroadcastID
	msg.isBroadcastClient = in.Metadata.BroadcastMsg.IsBroadcastClient
	msg.originAddr = in.Metadata.BroadcastMsg.OriginAddr
	msg.originMethod = in.Metadata.BroadcastMsg.OriginMethod
	if msg.originAddr == "" && msg.isBroadcastClient {
		msg.sendFn = createSendFn(in.Metadata.MessageID, in.Metadata.Method, finished, ctx)
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

func (srv *broadcastServer) processRequest2(msg content2) error {
	return srv.state.process(msg)
	//new, rC := srv.state.addOrUpdate2(in.Metadata.BroadcastMsg.BroadcastID)

	/*exists, rC := srv.state.get2(in.Metadata.BroadcastMsg.BroadcastID)
	new := false
	if !exists {
		// slow path
		new, rC = srv.state.add2(in.Metadata.BroadcastMsg.BroadcastID)
	}

	//rC.Do(func() {
	//go handleReq(srv.router, in.Metadata.BroadcastMsg.BroadcastID, rC, msg)
	//})
	if new {
		go handleReq(srv.router, in.Metadata.BroadcastMsg.BroadcastID, rC, msg)
	}
	receiveChan := make(chan error)
	msg.receiveChan = receiveChan
	select {
	case rC.sendChan <- msg:
	case <-rC.ctx.Done():
		return errors.New("req is done")
	}
	return <-receiveChan*/
}

func handleReq(router IBroadcastRouter, broadcastID uint64, init *reqContent, msg content2) {
	done := false
	sent := false
	methods := make([]string, 0, 3)
	var respErr error
	var respMsg protoreflect.ProtoMessage
	go router.CreateConnection(msg.originAddr)
	defer func() {
		done = true
		init.cancelFunc()
	}()
	for {
		select {
		case <-init.ctx.Done():
			return
		case bMsg := <-init.broadcastChan:
			if broadcastID != bMsg.broadcastID {
				//bMsg.receiveChan <- errors.New("wrong broadcastID")
				continue
			}
			if done {
				//bMsg.receiveChan <- errors.New("request is done and handled")
				continue
			}
			if bMsg.broadcast {
				// check if msg has already been broadcasted for this method
				if alreadyBroadcasted(methods, bMsg.method) {
					continue
				}
				err := router.Send(broadcastID, msg.originAddr, msg.originMethod, bMsg.msg)
				if err != nil {
					//bMsg.receiveChan <- err
					continue
				}
				methods = append(methods, bMsg.method)
				//slog.Info("methods", "methods", methods)
			} else {
				var err error
				if msg.sendFn != nil {
					err = msg.send(bMsg.reply.response, bMsg.reply.err)
				} else {
					//slog.Error("error", "bID", broadcastID)
					err = router.Send(broadcastID, msg.originAddr, msg.originMethod, bMsg.reply)
				}
				if err != nil {
					//slog.Error("error", "err", err)
					// add response if not already done
					if respMsg == nil {
						//slog.Info("resp is nil")
						//if msg.resp == nil {
						//msg.resp = bMsg.reply.response
						//msg.err = bMsg.reply.err
						respMsg = bMsg.reply.response
						respErr = bMsg.reply.err
						sent = true
					}
					//if err == nil {
					//slog.Error("added response", "resp", bMsg.reply.response)
					//}
					//bMsg.receiveChan <- err
					continue
				}
				//slog.Error("done", "bID", broadcastID)
				return
			}
		case new := <-init.sendChan:
			if done {
				new.receiveChan <- errors.New("req is done in handler")
				continue
			}
			if msg.originAddr == "" && new.originAddr != "" {
				msg.originAddr = new.originAddr
			}
			if msg.originMethod == "" && new.originMethod != "" {
				msg.originMethod = new.originMethod
			}
			if msg.sendFn == nil && new.sendFn != nil {
				msg.sendFn = new.sendFn
			}
			if sent && msg.sendFn != nil {
				//slog.Error("can be routed")
				err := msg.send(respMsg, respErr)
				//err := srv.router.send(broadcastID, msg, msg.resp)
				if err != nil {
					//slog.Info("error when sending", "err", err)
					new.receiveChan <- err
					continue
				}
				//slog.Error("was routed")
				new.receiveChan <- nil
				return
			}
			new.receiveChan <- nil
		}
	}
}

func (srv *broadcastServer) createRequest(ctx ServerCtx, in *Message, finished chan<- *Message) (*content, error) {
	addOriginMethod(in.Metadata)
	data, err := srv.state.newData(ctx.Context, in.Metadata.BroadcastMsg.IsBroadcastClient, in.Metadata.BroadcastMsg.OriginAddr, in.Metadata.BroadcastMsg.OriginMethod, in.Metadata.MessageID, in.Metadata.Method, finished)
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

func (srv *broadcastServer) checkMsgAlreadyProcessed(broadcastID uint64) (bool, error) {
	data, err := srv.state.get(broadcastID)
	if err != nil {
		return true, err
	}
	if data.canBeRouted() {
		err = srv.router.SendOrg(broadcastID, data, data.getResponse())
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
	if !md.BroadcastMsg.IsBroadcastClient {
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
	if in.Metadata.BroadcastMsg.BroadcastID == 0 {
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
	if VERSION == 1 {
		srv.broadcastHandler1(method, req, broadcastID, opts...)
	} else {
		srv.broadcastHandler2(method, req, broadcastID, opts...)
	}
}

func (srv *broadcastServer) sendToClientHandler(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	if VERSION == 1 {
		srv.sendToClientHandler1(broadcastID, resp, err)
	} else {
		srv.sendToClientHandler2(broadcastID, resp, err)
	}
}

func (srv *broadcastServer) broadcastHandler2(method string, req protoreflect.ProtoMessage, broadcastID uint64, opts ...broadcast.BroadcastOptions) {
	srv.state.processBroadcast(broadcastID, req, method)
	//rc, err := srv.state.getReqContent(broadcastID)
	//if err != nil {
	//return
	//}
	//select {
	//case rc.broadcastChan <- bMsg{
	//broadcast:   true,
	//msg:         newBroadcastMessage2(broadcastID, req, method),
	//method:      method,
	//broadcastID: broadcastID,
	//}:
	//case <-rc.ctx.Done():
	//}
}

func (srv *broadcastServer) sendToClientHandler2(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	srv.state.processSendToClient(broadcastID, resp, err)
	//rc, err := srv.state.getReqContent(broadcastID)
	//if err != nil {
	//return
	//}
	//select {
	//case rc.broadcastChan <- bMsg{
	//reply: &reply{
	//response: resp,
	//err:      err,
	//},
	//broadcastID: broadcastID,
	//}:
	//case <-rc.ctx.Done():
	//}
}

func (srv *broadcastServer) broadcastHandler1(method string, req RequestTypes, broadcastID uint64, opts ...broadcast.BroadcastOptions) {
	options := broadcast.BroadcastOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	srv.broadcast(newBroadcastMessage(broadcastID, req, method, options))

	// not broadcasting in a goroutine can lead to deadlock. All handlers are run sync
	// and thus the server have to return from the handler in order to process the next
	// request.
	//<-finished
	//}
}

func (srv *broadcastServer) sendToClientHandler1(broadcastID uint64, resp protoreflect.ProtoMessage, err error) {
	srv.sendToClient(newReply(resp, err, broadcastID))
}

func (srv *broadcastServer) forwardHandler(req RequestTypes, method string, broadcastID uint64, forwardAddr, originAddr string) {
	cd := broadcastCallData{
		Message:           req,
		Method:            method,
		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        originAddr,
		ServerAddresses:   []string{forwardAddr},
	}
	srv.viewMutex.RLock()
	// drop request if a view change has occured
	srv.view.broadcastCall(context.Background(), cd)
	srv.viewMutex.RUnlock()

}

func (srv *Server) SendToClientHandler(resp protoreflect.ProtoMessage, err error, broadcastID uint64) {
	srv.broadcastSrv.sendToClientHandler(broadcastID, resp, err)
}

func (srv *broadcastServer) registerBroadcastFunc(method string) {
	srv.router.AddServerHandler(method, func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options broadcast.BroadcastOptions, id uint32, addr string) {
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
	})
}

func (srv *broadcastServer) registerSendToClientHandler(method string, handler clientHandler) {
	srv.router.AddClientHandler(method, handler)
}

func (srv *Server) RegisterConfig(config RawConfiguration) {
	// temporarily stop the broadcast server to prevent queueing
	// broadcast messages. Otherwise, beacuse the broadcast queueing
	// method holds a read lock and can thus prevent this method
	// from running.
	// handle all queued broadcast messages before changing the view
	srv.broadcastSrv.viewMutex.Lock()
	// delete all client requests. This resets all broadcast requests.
	//srv.broadcastSrv.state.prune()
	srv.broadcastSrv.view = config
	srv.broadcastSrv.viewMutex.Unlock()
}
