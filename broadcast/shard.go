package broadcast

import (
	"context"
	"time"
)

const NumShards = 16

type shardMetrics struct {
	totalMsgs            uint64
	numMsgs              uint64
	droppedMsgs          uint64
	numBroadcastMsgs     uint64
	droppedBroadcastMsgs uint64
	numReqs              uint64
	finishedReqs         uint64
	lifetimes            [][]time.Time
	avgLifetime          time.Duration
	minLifetime          time.Duration
	maxLifetime          time.Duration
}

type shard struct {
	id            int
	sendChan      chan Content
	broadcastChan chan Msg
	ctx           context.Context
	cancelFunc    context.CancelFunc
	metrics       shardMetrics
	reqs          map[uint64]*BroadcastRequest
	router        Router

	preserveOrdering bool
}

func createShards(ctx context.Context, shardBuffer int, router Router) []*shard {
	shards := make([]*shard, NumShards)
	for i := range shards {
		ctx, cancel := context.WithCancel(ctx)
		shards[i] = &shard{
			id:            i,
			sendChan:      make(chan Content, shardBuffer),
			broadcastChan: make(chan Msg, shardBuffer),
			ctx:           ctx,
			cancelFunc:    cancel,
			reqs:          make(map[uint64]*BroadcastRequest, shardBuffer),
			router:        router,
		}
	}
	return shards
}

func (s *shard) run(reqTTL time.Duration, sendBuffer int) {
	for {
		select {
		case <-s.ctx.Done():
			s.reqs = nil
			return
		case msg := <-s.sendChan:
			s.metrics.numMsgs++
			//if metrics != nil {
			//metrics.AddShardDistribution(s.id)
			//}
			if req, ok := s.reqs[msg.BroadcastID]; ok {
				// must check if the req is done first to prevent
				// unecessarily running the server handler
				select {
				case <-req.ctx.Done():
					s.metrics.droppedMsgs++
					msg.ReceiveChan <- shardResponse{
						err: AlreadyProcessedErr{},
					}
				default:
				}
				if msg.IsCancellation {
					req.cancellationCtxCancel()
					msg.ReceiveChan <- shardResponse{
						err: nil,
					}
					continue
				}
				if !msg.IsBroadcastClient && !s.preserveOrdering {
					// no need to send it to the broadcast request goroutine.
					// the first request should contain all info needed
					// except for the routing info given in the client req.
					msg.ReceiveChan <- shardResponse{
						err:    nil,
						reqCtx: req.cancellationCtx,
					}
					continue
				}
				// msg.Ctx will correspond to the streamCtx between the client and this server.
				// We can thus listen to it and signal a cancellation if the client goes offline
				// or cancels the request. We also have to listen to the req.ctx to prevent leaking
				// the goroutine.
				go func() {
					select {
					case <-req.ctx.Done():
					case <-msg.Ctx.Done():
					}
					req.cancellationCtxCancel()
				}()
				// must check if the req is done to prevent deadlock
				select {
				case <-req.ctx.Done():
					s.metrics.droppedMsgs++
					msg.ReceiveChan <- shardResponse{
						err: AlreadyProcessedErr{},
					}
				case req.sendChan <- msg:
				}
			} else {
				if msg.IsCancellation {
					// ignore cancellations if a broadcast request
					// has not been created yet
					continue
				}
				if msg.IsBroadcastClient {
					// msg.Ctx will correspond to the streamCtx between the client and this server,
					// meaning the ctx will cancel when the client cancels or disconnects.
					msg.Ctx, msg.CancelCtx = context.WithCancel(msg.Ctx)
				} else {
					msg.Ctx, msg.CancelCtx = context.WithCancel(context.Background())
				}
				// check size of s.reqs. If too big, then perform necessary cleanup.
				// should only affect the current shard and not the others.
				ctx, cancel := context.WithTimeout(s.ctx, reqTTL)
				req := &BroadcastRequest{
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
					broadcastChan:         make(chan Msg, sendBuffer),
					started:               time.Now(),
					cancellationCtx:       msg.Ctx,
					cancellationCtxCancel: msg.CancelCtx,
				}
				s.reqs[msg.BroadcastID] = req
				go req.handle(s.router, msg.BroadcastID, msg)
				select {
				case <-req.ctx.Done():
					s.metrics.droppedMsgs++
					msg.ReceiveChan <- shardResponse{
						err: AlreadyProcessedErr{},
					}
				case req.sendChan <- msg:
				}
			}
		case msg := <-s.broadcastChan:
			s.metrics.numBroadcastMsgs++
			if req, ok := s.reqs[msg.BroadcastID]; ok {
				if msg.Cancellation != nil {
					if msg.Cancellation.end {
						req.cancelFunc()
						continue
					}
					if !req.sentCancellation {
						req.sentCancellation = true
						go s.router.Send(msg.BroadcastID, "", "", msg.Cancellation)
					}
					continue
				}
				select {
				case <-req.ctx.Done():
					s.metrics.droppedBroadcastMsgs++
				case req.broadcastChan <- msg:
				}
			}
		}
	}
}

func (s *shard) Close() {
	s.cancelFunc()
}

func (s *shard) getStats() shardMetrics {
	s.metrics.numReqs = uint64(len(s.reqs))
	s.metrics.lifetimes = make([][]time.Time, len(s.reqs))
	s.metrics.totalMsgs = s.metrics.numBroadcastMsgs + s.metrics.numMsgs
	minLifetime := 100 * time.Hour
	maxLifetime := time.Duration(0)
	totalLifetime := time.Duration(0)
	i := 0
	for _, req := range s.reqs {
		select {
		case <-req.ctx.Done():
			s.metrics.finishedReqs++
		default:
		}
		s.metrics.lifetimes[i] = []time.Time{req.started, req.ended}
		lifetime := req.ended.Sub(req.started)
		if lifetime > 0 {
			if lifetime < minLifetime {
				minLifetime = lifetime
			}
			if lifetime > maxLifetime {
				maxLifetime = lifetime
			}
			totalLifetime += lifetime
		}
		i++
	}
	s.metrics.minLifetime = minLifetime
	s.metrics.maxLifetime = maxLifetime
	s.metrics.avgLifetime = totalLifetime
	return s.metrics
}
