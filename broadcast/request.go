package broadcast

import (
	"context"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastRequest struct {
	broadcastChan chan Msg
	sendChan      chan Content
	ctx           context.Context
	cancelFunc    context.CancelFunc
	started       time.Time
	ended         time.Time
	//sync.Once
}

// func (req *BroadcastRequest) handle(router *BroadcastRouter, broadcastID uint64, msg Content, metrics *Metric) {
func (req *BroadcastRequest) handle(router Router, broadcastID uint64, msg Content) {
	//start := time.Now()
	sent := false
	methods := make([]string, 0, 3)
	var respErr error
	var respMsg protoreflect.ProtoMessage
	//if metrics != nil {
	//metrics.AddGoroutine(broadcastID, "req")
	//}
	//go router.CreateConnection(msg.OriginAddr)
	defer func() {
		req.ended = time.Now()
		//if metrics != nil {
		//metrics.AddRoundTripLatency(start)
		//metrics.RemoveGoroutine(broadcastID, "req")
		//}
		req.cancelFunc()
	}()
	for {
		select {
		case <-req.ctx.Done():
			//if metrics != nil {
			//metrics.AddFinishedFailed()
			//}
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
				err := router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Msg)
				if err != nil {
					continue
				}
				methods = append(methods, bMsg.Method)
			} else {
				// BroadcastCall if origin addr is non-empty.
				if msg.isBroadcastCall() {
					_ = router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Reply)
					//if err != nil && router.logger != nil {
					//router.logger.Error("broadcast: could not send response to client", "err", err, "broadcastID", broadcastID)
					//}
					return
				}
				// QuorumCall if origin addr is empty.
				//if !msg.hasReceivedClientRequest() {
				//// Has not received client request
				//continue
				//}
				// Has received client request
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
				new.ReceiveChan <- BroadcastIDErr{}
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
				if err != nil {
					new.ReceiveChan <- err
					// should return here?
					return
				}
				//new.ReceiveChan <- errors.New("req is done and should be returned immediately to client")
				new.ReceiveChan <- AlreadyProcessedErr{}
				return
			}
			new.ReceiveChan <- nil
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
