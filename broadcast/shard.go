package broadcast

import (
	"context"
	"sync"
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
	mut           sync.RWMutex
	id            int
	sendChan      chan Content
	broadcastChan chan Msg
	ctx           context.Context
	cancelFunc    context.CancelFunc
	metrics       shardMetrics
	//reqs          map[uint64]*BroadcastRequest
	reqs   map[uint64]*BroadcastProcessor
	reqTTL time.Duration
	router Router
	nextGC time.Time

	preserveOrdering bool
	order            map[string]int
}

func createShards(ctx context.Context, shardBuffer int, router Router, order map[string]int, reqTTL time.Duration) []*shard {
	shards := make([]*shard, NumShards)
	for i := range shards {
		ctx, cancel := context.WithCancel(ctx)
		shards[i] = &shard{
			id:            i,
			sendChan:      make(chan Content, shardBuffer),
			broadcastChan: make(chan Msg, shardBuffer),
			ctx:           ctx,
			cancelFunc:    cancel,
			//reqs:             make(map[uint64]*BroadcastRequest, shardBuffer),
			reqs:             make(map[uint64]*BroadcastProcessor, shardBuffer),
			reqTTL:           reqTTL,
			router:           router,
			preserveOrdering: order != nil,
			order:            order,
		}
	}
	return shards
}

func (s *shard) run(sendBuffer int) {
	for {
		select {
		case <-s.ctx.Done():
			// it is possible that GC is running concurrently in another goroutine
			s.mut.Lock()
			s.reqs = nil
			s.mut.Unlock()
			return
		case msg := <-s.sendChan:
			s.metrics.numMsgs++
			if req, ok := s.getProcessor(msg.BroadcastID); ok {
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
						err:              nil,
						reqCtx:           req.cancellationCtx,
						enqueueBroadcast: req.enqueueBroadcast,
					}
					continue
				}
				if msg.IsBroadcastClient {
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
				}
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
				s.addProcessor(sendBuffer, msg)
			}
		case msg := <-s.broadcastChan:
			s.metrics.numBroadcastMsgs++
			if req, ok := s.getProcessor(msg.BroadcastID); ok {
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

func (s *shard) getProcessor(broadcastID uint64) (*BroadcastProcessor, bool) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	p, ok := s.reqs[broadcastID]
	return p, ok
}

func (s *shard) addProcessor(sendBuffer int, msg Content) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	if time.Since(s.nextGC) > 0 {
		// make sure the current request is done before running the GC.
		// This is to prevent running the GC in vain.
		t := s.reqTTL + 5*time.Second
		s.nextGC = time.Now().Add(t)
		go s.gc(t)
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
	ctx, cancel := context.WithTimeout(s.ctx, s.reqTTL)
	//req := &BroadcastRequest{
	req := &BroadcastProcessor{
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
		router:                s.router,
		cancellationCtx:       msg.Ctx,
		cancellationCtxCancel: msg.CancelCtx,
		executionOrder:        s.order,
	}
	s.reqs[msg.BroadcastID] = req
	//go req.handle(s.router, msg.BroadcastID, msg)
	go req.handle(msg)
	select {
	case <-req.ctx.Done():
		s.metrics.droppedMsgs++
		msg.ReceiveChan <- shardResponse{
			err: AlreadyProcessedErr{},
		}
	case req.sendChan <- msg:
	}
}

func (s *shard) gc(nextGC time.Duration) {
	// make sure there is overlap between GC's
	time.Sleep(nextGC + 1*time.Second)
	s.mut.Lock()
	defer s.mut.Unlock()
	newReqs := make(map[uint64]*BroadcastProcessor)
	for broadcastID, req := range s.reqs {
		// stale requests should be cancelled and removed immediately
		if time.Since(req.started) > s.reqTTL {
			req.cancelFunc()
			continue
		}
		select {
		case <-req.ctx.Done():
			// if the request has finished early then it has
			// probably executed successfully on all servers.
			// we can thus assume it is safe to remove the req
			// after a short period after it has finished because
			// it will likely not receive any msg related to this
			// broadcast request.
			if time.Since(req.ended) > s.reqTTL/5 {
				continue
			}
		default:
		}
		newReqs[broadcastID] = req
	}
	s.reqs = newReqs
}

func (s *shard) getStats() shardMetrics {
	s.mut.RLock()
	defer s.mut.RUnlock()
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
