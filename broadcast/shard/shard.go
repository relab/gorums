package shard

import (
	"context"
	"github.com/relab/gorums/broadcast/processor"
	"github.com/relab/gorums/broadcast/router"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/logging"
)

type Shard struct {
	mut         sync.RWMutex
	id          uint16
	parentCtx   context.Context
	processors  map[uint64]*processor.Processor
	reqTTL      time.Duration
	router      router.Router
	nextGC      time.Time
	shardBuffer int
	sendBuffer  int
	logger      *slog.Logger

	preserveOrdering bool
	order            map[string]int
}

type Config struct {
	Id          uint16
	ParentCtx   context.Context
	ShardBuffer int
	SendBuffer  int
	ReqTTL      time.Duration
	Router      router.Router
	Order       map[string]int
	Logger      *slog.Logger
}

func New(config *Config) *Shard {
	return &Shard{
		id:               config.Id,
		parentCtx:        config.ParentCtx,
		processors:       make(map[uint64]*processor.Processor, config.ShardBuffer),
		shardBuffer:      config.ShardBuffer,
		sendBuffer:       config.SendBuffer,
		reqTTL:           config.ReqTTL,
		router:           config.Router,
		preserveOrdering: config.Order != nil,
		order:            config.Order,
		logger:           config.Logger,
	}
}

func (s *Shard) HandleMsg(msg *processor.RequestDto) {
	// Optimization: first check with a read lock if the processor already exists
	if p, ok := s.getProcessor(msg.BroadcastID); ok {
		s.process(p, msg)
		return
	}
	proc, alreadyExists := s.addProcessor(msg)
	if alreadyExists {
		s.process(proc, msg)
	}
}

func (s *Shard) process(p *processor.Processor, msg *processor.RequestDto) {
	// must check if the processor is done first to prevent unnecessarily running the server handler.
	if p.IsFinished(msg) {
		return
	}
	p.EnqueueExternalMsg(msg)
}

func (s *Shard) getProcessor(broadcastID uint64) (*processor.Processor, bool) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	p, ok := s.processors[broadcastID]
	return p, ok
}

func (s *Shard) addProcessor(msg *processor.RequestDto) (*processor.Processor, bool) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if p, ok := s.processors[msg.BroadcastID]; ok {
		return p, true
	}
	//if time.Since(s.nextGC) > 0 {
	//	// make sure the current request is done before running the GC.
	//	// This is to prevent running the GC in vain.
	//	t := s.reqTTL + 5*time.Second
	//	s.nextGC = time.Now().Add(t)
	//	go s.gc(t)
	//}
	if !msg.IsServer {
		// msg.Ctx will correspond to the streamCtx between the client and this server,
		// meaning the ctx will cancel when the client cancels or disconnects.
		msg.Ctx, msg.CancelCtx = context.WithCancel(msg.Ctx)
	} else {
		msg.Ctx, msg.CancelCtx = context.WithCancel(context.Background())
	}
	// check size of s.reqs. If too big, then perform necessary cleanup.
	// should only affect the current Shard and not the others.
	ctx, cancel := context.WithTimeout(s.parentCtx, s.reqTTL)
	var logger *slog.Logger
	if s.logger != nil {
		logger = s.logger.With(logging.BroadcastID(msg.BroadcastID))
	}
	config := &processor.Config{
		Ctx:             ctx,
		CancelFunc:      cancel,
		SendBuffer:      s.sendBuffer,
		Router:          s.router,
		ExecutionOrder:  s.order,
		Logger:          logger,
		BroadcastID:     msg.BroadcastID,
		OriginAddr:      msg.OriginAddr,
		OriginMethod:    msg.OriginMethod,
		IsServer:        msg.IsServer,
		SendFn:          msg.SendFn,
		OriginDigest:    msg.OriginDigest,
		OriginSignature: msg.OriginSignature,
		OriginPubKey:    msg.OriginPubKey,
	}
	proc := processor.New(config)
	s.processors[msg.BroadcastID] = proc
	go proc.Start(msg)
	return proc, false
}

/*func (s *Shard) gc(nextGC time.Duration) {
	// make sure there is overlap between GC's
	time.Sleep(nextGC + 1*time.Second)
	s.mut.Lock()
	defer s.mut.Unlock()
	newReqs := make(map[uint64]*processor.Processor, s.shardBuffer)
	for broadcastID, proc := range s.processors {
		// stale requests should be cancelled and removed immediately
		if time.Since(proc.started) > s.reqTTL {
			proc.cancelFunc()
			continue
		}
		select {
		case <-proc.ctx.Done():
			// if the request has finished early then it has
			// probably executed successfully on all servers.
			// we can thus assume it is safe to remove the req
			// after a short period after it has finished because
			// it will likely not receive any msg related to this
			// broadcast request.
			if time.Since(proc.ended) > s.reqTTL/5 {
				continue
			}
		default:
		}
		newReqs[broadcastID] = proc
	}
	s.processors = newReqs
}*/

func (s *Shard) log(msg string, err error, args ...slog.Attr) {
	if s.logger != nil {
		args = append(args, logging.Err(err), logging.Type("shard"))
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}
		s.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}
