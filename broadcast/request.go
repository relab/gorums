package broadcast

import (
	"context"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastRequest struct {
	broadcastChan         chan Msg
	sendChan              chan Content
	ctx                   context.Context
	cancelFunc            context.CancelFunc
	started               time.Time
	ended                 time.Time
	cancellationCtx       context.Context
	cancellationCtxCancel context.CancelFunc // should only be called by the shard
	sentCancellation      bool

	executionOrder []string
	orderIndex     int
	outOfOrderMsgs map[string][]Content
}

// func (req *BroadcastRequest) handle(router *BroadcastRouter, broadcastID uint64, msg Content, metrics *Metric) {
func (req *BroadcastRequest) handle(router Router, broadcastID uint64, msg Content) {
	sent := false
	methods := make([]string, 0, 3)
	req.initOrder()
	var respErr error
	var respMsg protoreflect.ProtoMessage
	// connect to client immediately to potentially save some time
	go router.Connect(msg.OriginAddr)
	defer func() {
		req.ended = time.Now()
		req.cancelFunc()
		req.cancellationCtxCancel()
	}()
	for {
		select {
		case <-req.ctx.Done():
			return
		case bMsg := <-req.broadcastChan:
			if broadcastID != bMsg.BroadcastID {
				continue
			}
			if bMsg.Broadcast {
				// check if msg has already been broadcasted for this method
				if alreadyBroadcasted(methods, bMsg.Method) {
					continue
				}
				router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Msg)
				/*err := router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Msg)
				if err != nil {
					continue
				}*/
				methods = append(methods, bMsg.Method)
				req.updateOrder(bMsg.Method)
			} else {
				// BroadcastCall if origin addr is non-empty.
				if msg.isBroadcastCall() {
					go router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Reply)
					//_ = router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Reply)
					//if err != nil && router.logger != nil {
					//router.logger.Error("broadcast: could not send response to client", "err", err, "broadcastID", broadcastID)
					//}
					return
				}
				// QuorumCall if origin addr is empty.
				err := msg.send(bMsg.Reply.Response, bMsg.Reply.Err)
				if err != nil {
					// add response if not already done
					if respMsg == nil {
						respMsg = bMsg.Reply.Response
						respErr = bMsg.Reply.Err
						sent = true
					}
					continue
				}
				//if metrics != nil {
				//metrics.AddFinishedSuccessful()
				//}
				return
			}
		case new := <-req.sendChan:
			if new.BroadcastID != broadcastID {
				new.ReceiveChan <- shardResponse{
					err: BroadcastIDErr{},
				}
				continue
			}
			if new.IsCancellation {
				new.ReceiveChan <- shardResponse{
					err: nil,
				}
				continue
			}
			if msg.OriginAddr == "" && new.OriginAddr != "" {
				msg.OriginAddr = new.OriginAddr
			}
			if msg.OriginMethod == "" && new.OriginMethod != "" {
				msg.OriginMethod = new.OriginMethod
			}
			if msg.SendFn == nil && new.SendFn != nil {
				msg.SendFn = new.SendFn
				msg.IsBroadcastClient = new.IsBroadcastClient
			}
			// this only pertains to requests where the server has a
			// direct connection to the client, e.g. QuorumCall.
			if sent && !msg.isBroadcastCall() {
				err := msg.send(respMsg, respErr)
				if err == nil {
					err = AlreadyProcessedErr{}
				}
				new.ReceiveChan <- shardResponse{
					err: err,
				}
				return
			}
			if !req.isInOrder(new.CurrentMethod) {
				// save the message and execute it later
				req.addToOutOfOrder(new)
				new.ReceiveChan <- shardResponse{
					err: OutOfOrderErr{},
				}
			}
			new.ReceiveChan <- shardResponse{
				err:    nil,
				reqCtx: req.cancellationCtx,
			}
		}
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

func (c *Content) isBroadcastCall() bool {
	return c.OriginAddr != ""
}

func (c *Content) hasReceivedClientRequest() bool {
	return c.IsBroadcastClient && c.SendFn != nil
}

func (r *BroadcastRequest) initOrder() {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	r.outOfOrderMsgs = make(map[string][]Content)
}

func (r *BroadcastRequest) isInOrder(method string) bool {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return true
	}
	// the first method should always be allowed to be executed
	if r.executionOrder[0] == method {
		return true
	}

	return false
}

func (r *BroadcastRequest) addToOutOfOrder(msg Content) {

}

func (r *BroadcastRequest) updateOrder(method string) {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	for i, m := range r.executionOrder {
		if m == method {
			if i > r.orderIndex {
				r.orderIndex = i
			}
			return
		}
	}
}
