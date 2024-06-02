package broadcast

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/logging"
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
	mut sync.RWMutex
	id  int
	//sendChan      chan Content
	//broadcastChan chan Msg
	parentCtx context.Context
	metrics   shardMetrics
	//reqs          map[uint64]*BroadcastRequest
	reqs        map[uint64]*BroadcastProcessor
	reqTTL      time.Duration
	router      Router
	nextGC      time.Time
	shardBuffer int
	sendBuffer  int
	logger      *slog.Logger

	preserveOrdering bool
	order            map[string]int
}

func createShards(ctx context.Context, shardBuffer, sendBuffer int, router Router, order map[string]int, reqTTL time.Duration, logger *slog.Logger) []*shard {
	shards := make([]*shard, NumShards)
	for i := range shards {
		shards[i] = &shard{
			id: i,
			//sendChan:      make(chan Content, shardBuffer),
			//broadcastChan: make(chan Msg, shardBuffer),
			parentCtx: ctx,
			//reqs:             make(map[uint64]*BroadcastRequest, shardBuffer),
			reqs:             make(map[uint64]*BroadcastProcessor, shardBuffer),
			shardBuffer:      shardBuffer,
			sendBuffer:       sendBuffer,
			reqTTL:           reqTTL,
			router:           router,
			preserveOrdering: order != nil,
			order:            order,
			logger:           logger,
		}
	}
	return shards
}

func (s *shard) handleMsg(msg *Content) shardResponse {
	//s.metrics.numMsgs++
	// Optimization: first check with a read lock if the processor already exists
	if p, ok := s.getProcessor(msg.BroadcastID); ok {
		return s.process(p, msg)
	} else {
		if msg.IsCancellation {
			// ignore cancellations if a broadcast request
			// has not been created yet
			return shardResponse{
				err: MissingReqErr{},
			}
		}
		proc, alreadyExists := s.addProcessor(msg)
		if alreadyExists {
			return s.process(proc, msg)
		}
		select {
		case resp := <-msg.ReceiveChan:
			return resp
		case <-proc.ctx.Done():
			return shardResponse{
				err: ReqFinishedErr{},
			}
		}
	}
}

func (s *shard) process(p *BroadcastProcessor, msg *Content) shardResponse {
	// must check if the req is done first to prevent
	// unecessarily running the server handler.
	select {
	case <-p.ctx.Done():
		p.log("msg: already processed", AlreadyProcessedErr{}, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
		return shardResponse{
			err: AlreadyProcessedErr{},
		}
	default:
	}
	if msg.IsCancellation {
		p.cancellationCtxCancel()
		return shardResponse{
			err: nil,
		}
	}
	if !msg.IsBroadcastClient && !s.preserveOrdering {
		p.log("msg: processed", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
		// no need to send it to the broadcast request goroutine.
		// the first request should contain all info needed
		// except for the routing info given in the client req.
		return shardResponse{
			err:              nil,
			reqCtx:           p.cancellationCtx,
			enqueueBroadcast: p.enqueueBroadcast,
		}
	}
	// must check if the req is done to prevent deadlock
	select {
	case <-p.ctx.Done():
		//s.metrics.droppedMsgs++
		return shardResponse{
			err: AlreadyProcessedErr{},
		}
	case p.sendChan <- msg:
		select {
		case resp := <-msg.ReceiveChan:
			return resp
		case <-p.ctx.Done():
			return shardResponse{
				err: ReqFinishedErr{},
			}
		}
	}

}

func (s *shard) handleBMsg(msg *Msg) {
	//s.metrics.numBroadcastMsgs++
	if req, ok := s.getProcessor(msg.BroadcastID); ok {
		select {
		case <-req.ctx.Done():
			//s.metrics.droppedBroadcastMsgs++
		case req.broadcastChan <- msg:
		}
	}
}

func (s *shard) getProcessor(broadcastID uint64) (*BroadcastProcessor, bool) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	p, ok := s.reqs[broadcastID]
	return p, ok
}

func (s *shard) addProcessor(msg *Content) (*BroadcastProcessor, bool) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if p, ok := s.reqs[msg.BroadcastID]; ok {
		return p, true
	}
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
	ctx, cancel := context.WithTimeout(s.parentCtx, s.reqTTL)
	//req := &BroadcastRequest{
	var logger *slog.Logger
	if s.logger != nil {
		logger = s.logger.With(logging.BroadcastID(msg.BroadcastID))
	}
	req := &BroadcastProcessor{
		ctx:                   ctx,
		cancelFunc:            cancel,
		sendChan:              make(chan *Content, s.sendBuffer),
		broadcastChan:         make(chan *Msg, s.sendBuffer),
		started:               time.Now(),
		router:                s.router,
		cancellationCtx:       msg.Ctx,
		cancellationCtxCancel: msg.CancelCtx,
		executionOrder:        s.order,
		logger:                logger,
	}
	s.reqs[msg.BroadcastID] = req
	go req.handle(msg)
	return req, false
}

func (s *shard) gc(nextGC time.Duration) {
	// make sure there is overlap between GC's
	time.Sleep(nextGC + 1*time.Second)
	s.mut.Lock()
	defer s.mut.Unlock()
	newReqs := make(map[uint64]*BroadcastProcessor, s.shardBuffer)
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
