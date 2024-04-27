package broadcast

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const NumShards = 16

type shardElement struct {
	id            int
	sendChan      chan Content
	broadcastChan chan Msg
	ctx           context.Context
	cancelFunc    context.CancelFunc
	//sync.Once
}

func createShards(ctx context.Context, shardBuffer int) []*shardElement {
	shards := make([]*shardElement, NumShards)
	for i := range shards {
		ctx, cancel := context.WithCancel(ctx)
		shards[i] = &shardElement{
			id:            i,
			sendChan:      make(chan Content, shardBuffer),
			broadcastChan: make(chan Msg, shardBuffer),
			ctx:           ctx,
			cancelFunc:    cancel,
		}
	}
	return shards
}

func (shard *shardElement) run(router *BroadcastRouter, reqTTL time.Duration, sendBuffer, shardBuffer int, metrics *Metric) {
	reqs := make(map[uint64]*reqContent, shardBuffer)
	for {
		select {
		case <-shard.ctx.Done():
			return
		case msg := <-shard.sendChan:
			if metrics != nil {
				metrics.AddShardDistribution(shard.id)
			}
			if req, ok := reqs[msg.BroadcastID]; ok {
				if !msg.IsBroadcastClient {
					// no need to send it to the broadcast request goroutine.
					// the first request should contain all info needed
					// except for the routing info given in the client req.
					msg.ReceiveChan <- nil
					continue
				}
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- fmt.Errorf("req is done. broadcastID: %v", msg.BroadcastID)
				case req.sendChan <- msg:
				}
			} else {
				ctx, cancel := context.WithTimeout(shard.ctx, reqTTL)
				req := &reqContent{
					ctx:        ctx,
					cancelFunc: cancel,
					// it is important to not buffer the channel. Otherwise,
					// it will cause a deadlock on the receiver. Will
					// happen in this scenario: A msg is queued/buffered but the
					// listening goroutine is stopped. This is because a req is
					// first processed by a shard and then sent to the goroutine.
					// the original sender is only listening on the receive channel
					// and not on the ctx of the request.
					sendChan: make(chan Content),
					// this channel can be buffered because there is no receive
					// channel. The result is simply ignored.
					broadcastChan: make(chan Msg, sendBuffer),
				}
				reqs[msg.BroadcastID] = req
				go handleReq(router, msg.BroadcastID, req, msg, metrics)
				select {
				case <-req.ctx.Done():
					msg.ReceiveChan <- errors.New("req is done")
				case req.sendChan <- msg:
				}
			}
		case msg := <-shard.broadcastChan:
			if metrics != nil {
				metrics.AddShardDistribution(shard.id)
			}
			if req, ok := reqs[msg.BroadcastID]; ok {
				select {
				case <-req.ctx.Done():
				case req.broadcastChan <- msg:
				}
			}
		}
	}
}

func (s *shardElement) Close() {
	s.cancelFunc()
}
