package broadcast

import (
	"errors"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func handleReq(router *BroadcastRouter, broadcastID uint64, init *reqContent, msg Content, metrics *Metric) {
	start := time.Now()
	done := false
	sent := false
	methods := make([]string, 0, 3)
	var respErr error
	var respMsg protoreflect.ProtoMessage
	if metrics != nil {
		metrics.AddGoroutine(broadcastID, "req")
	}
	//go router.CreateConnection(msg.OriginAddr)
	defer func() {
		if metrics != nil {
			metrics.AddRoundTripLatency(start)
			metrics.RemoveGoroutine(broadcastID, "req")
		}
		done = true
		init.cancelFunc()
	}()
	for {
		select {
		case <-init.ctx.Done():
			if metrics != nil {
				metrics.AddFinishedFailed()
			}
			return
		case bMsg := <-init.broadcastChan:
			if broadcastID != bMsg.BroadcastID {
				continue
			}
			if done {
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
				if msg.OriginAddr != "" {
					err := router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Reply)
					if err != nil {
						if metrics != nil {
							metrics.AddFinishedFailed()
						}
						if router.logger != nil {
							router.logger.Error("broadcast: could not send response to client", "err", err, "broadcastID", broadcastID)
						}
						//slog.Error("broadcast: could not send response to client", "err", err, "broadcastID", broadcastID)
					} else {
						if metrics != nil {
							metrics.AddFinishedSuccessful()
						}
						//slog.Info("broadcast: could send response to client", "err", err, "broadcastID", broadcastID)
					}
					return
				}
				// QuorumCall if origin addr is empty.
				if msg.SendFn == nil {
					// Has not received client request
					continue
				}
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
				if metrics != nil {
					metrics.AddFinishedSuccessful()
				}
				return
			}
		case new := <-init.sendChan:
			if done {
				new.ReceiveChan <- errors.New("req is done in handler")
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
			}
			if sent && msg.SendFn != nil {
				err := msg.send(respMsg, respErr)
				if err != nil {
					new.ReceiveChan <- err
					continue
				}
				//new.ReceiveChan <- errors.New("req is done and should be returned immediately to client")
				new.ReceiveChan <- nil
				if metrics != nil {
					metrics.AddFinishedSuccessful()
				}
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
