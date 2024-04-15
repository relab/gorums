package broadcast

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
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

type ServerHandler func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string)

// type serverHandler func(ctx context.Context, in RequestTypes, broadcastID, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string)
type ClientHandler func(broadcastID string, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error)

type reply struct {
	response    protoreflect.ProtoMessage
	err         error
	broadcastID string
	timestamp   time.Time
}

func HandleReq(router IBroadcastRouter, broadcastID string, init *reqContent, msg Content2) {
	done := false
	sent := false
	methods := make([]string, 0)
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
			//slog.Info("received bMsg")
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
			} else {
				var err error
				if msg.sendFn != nil {
					err = msg.send(bMsg.reply.response, bMsg.reply.err)
				} else {
					err = router.Send(broadcastID, msg.originAddr, msg.originMethod, bMsg.reply)
				}
				if err != nil {
					// add response if not already done
					if msg.resp == nil {
						msg.resp = bMsg.reply.response
						msg.err = bMsg.reply.err
						sent = true
					}
					//if err == nil {
					//slog.Error("added response", "resp", bMsg.reply.response)
					//}
					//bMsg.receiveChan <- err
					continue
				}
				return
			}
		case new := <-init.sendChan:
			if done {
				new.receiveChan <- errors.New("req is done")
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
				err := msg.retrySend()
				//err := srv.router.send(broadcastID, msg, msg.resp)
				if err != nil {
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

func alreadyBroadcasted(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

type bMsg struct {
	broadcast   bool
	broadcastID string
	msg         *broadcastMsg
	method      string
	reply       *reply
}

type broadcastMsg struct {
	request     protoreflect.ProtoMessage
	method      string
	broadcastID string
	options     BroadcastOptions
	ctx         context.Context
}

func (r *reply) getResponse() protoreflect.ProtoMessage {
	return r.response
}

func (r *reply) getError() error {
	return r.err
}

func (r *reply) getBroadcastID() string {
	return r.broadcastID
}
