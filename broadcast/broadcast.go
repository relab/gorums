package broadcast

import (
	"errors"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastOptions struct {
	ServerAddresses      []string
	GossipPercentage     float32
	TTL                  int
	Deadline             time.Time
	OmitUniquenessChecks bool
	SkipSelf             bool
}

func HandleReq(router IBroadcastRouter, broadcastID uint64, init *reqContent, msg Content2) {
	done := false
	sent := false
	methods := make([]string, 0, 3)
	var respErr error
	var respMsg protoreflect.ProtoMessage
	go router.CreateConnection(msg.OriginAddr)
	defer func() {
		done = true
		init.cancelFunc()
	}()
	for {
		select {
		case <-init.ctx.Done():
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
				var err error
				if msg.SendFn != nil {
					err = msg.send(bMsg.Reply.Response, bMsg.Reply.Err)
				} else {
					err = router.Send(broadcastID, msg.OriginAddr, msg.OriginMethod, bMsg.Reply)
				}
				if err != nil {
					// add response if not already done
					if respMsg == nil {
						respMsg = bMsg.Reply.Response
						respErr = bMsg.Reply.Err
						sent = true
					}
					continue
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
				new.ReceiveChan <- nil
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
