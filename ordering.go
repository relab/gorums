package gorums

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type gorumsStreamRequest struct {
	ctx  context.Context
	msg  *Message
	opts callOptions
}

type gorumsStreamResult struct {
	nid   uint32
	reply protoreflect.ProtoMessage
	err   error
}

type receiveQueue struct {
	msgID    uint64
	recvQ    map[uint64]chan *gorumsStreamResult
	recvQMut sync.RWMutex
}

func newReceiveQueue() *receiveQueue {
	return &receiveQueue{
		recvQ: make(map[uint64]chan *gorumsStreamResult),
	}
}

func (m *receiveQueue) nextMsgID() uint64 {
	return atomic.AddUint64(&m.msgID, 1)
}

func (m *receiveQueue) putChan(id uint64, c chan *gorumsStreamResult) {
	m.recvQMut.Lock()
	m.recvQ[id] = c
	m.recvQMut.Unlock()
}

func (m *receiveQueue) deleteChan(id uint64) {
	m.recvQMut.Lock()
	delete(m.recvQ, id)
	m.recvQMut.Unlock()
}

func (m *receiveQueue) putResult(id uint64, result *gorumsStreamResult) {
	m.recvQMut.RLock()
	c, ok := m.recvQ[id]
	m.recvQMut.RUnlock()
	if ok {
		c <- result
	}
}

type orderedNodeStream struct {
	*receiveQueue
	sendQ        chan gorumsStreamRequest
	node         *Node // needed for ID and setLastError
	backoff      backoff.Config
	rand         *rand.Rand
	gorumsClient ordering.GorumsClient
	gorumsStream ordering.Gorums_NodeStreamClient
	streamMut    sync.RWMutex
	streamBroken bool
	parentCtx    context.Context
	streamCtx    context.Context
	cancelStream context.CancelFunc
}

func (s *orderedNodeStream) connectOrderedStream(ctx context.Context, conn *grpc.ClientConn) error {
	var err error
	s.parentCtx = ctx
	s.streamCtx, s.cancelStream = context.WithCancel(s.parentCtx)
	s.gorumsClient = ordering.NewGorumsClient(conn)
	s.gorumsStream, err = s.gorumsClient.NodeStream(s.streamCtx)
	if err != nil {
		return err
	}
	go s.sendMsgs()
	go s.recvMsgs()
	return nil
}

func (s *orderedNodeStream) sendMsg(req gorumsStreamRequest) (err error) {
	s.streamMut.RLock()
	defer s.streamMut.RUnlock()

	c := make(chan struct{})

	// wait for either the message to be sent, or the request context being cancelled.
	// if the request context was cancelled, then we most likely have a blocked stream.
	go func() {
		select {
		case <-c:
		case <-req.ctx.Done():
			s.cancelStream()
		}
	}()

	err = s.gorumsStream.SendMsg(req.msg)
	if err != nil {
		s.node.setLastErr(err)
		s.streamBroken = true
	}
	c <- struct{}{}

	// unblock the waiting caller when sendAsync is not enabled
	if req.opts.callType == E_Multicast || req.opts.callType == E_Unicast && !req.opts.sendAsync {
		s.putResult(req.msg.Metadata.MessageID, &gorumsStreamResult{})
	}

	return err
}

func (s *orderedNodeStream) sendMsgs() {
	var req gorumsStreamRequest
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case req = <-s.sendQ:
		}
		// return error if stream is broken
		if s.streamBroken {
			err := status.Errorf(codes.Unavailable, "stream is down")
			s.putResult(req.msg.Metadata.MessageID, &gorumsStreamResult{nid: s.node.ID(), reply: nil, err: err})
			continue
		}
		// else try to send message
		err := s.sendMsg(req)
		if err != nil {
			// return the error
			s.putResult(req.msg.Metadata.MessageID, &gorumsStreamResult{nid: s.node.ID(), reply: nil, err: err})
		}
	}
}

func (s *orderedNodeStream) recvMsgs() {
	for {
		resp := newMessage(responseType)
		s.streamMut.RLock()
		err := s.gorumsStream.RecvMsg(resp)
		if err != nil {
			s.streamBroken = true
			s.streamMut.RUnlock()
			s.node.setLastErr(err)
			// attempt to reconnect
			s.reconnectStream()
		} else {
			s.streamMut.RUnlock()
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			s.putResult(resp.Metadata.MessageID, &gorumsStreamResult{nid: s.node.ID(), reply: resp.Message, err: err})
		}

		select {
		case <-s.parentCtx.Done():
			return
		default:
		}
	}
}

func (s *orderedNodeStream) reconnectStream() {
	s.streamMut.Lock()
	defer s.streamMut.Unlock()

	var retries float64
	for {
		var err error

		s.streamCtx, s.cancelStream = context.WithCancel(s.parentCtx)
		s.gorumsStream, err = s.gorumsClient.NodeStream(s.streamCtx)
		if err == nil {
			s.streamBroken = false
			return
		}
		s.cancelStream()
		s.node.setLastErr(err)
		delay := float64(s.backoff.BaseDelay)
		max := float64(s.backoff.MaxDelay)
		for r := retries; delay < max && r > 0; r-- {
			delay *= s.backoff.Multiplier
		}
		delay = math.Min(delay, max)
		delay *= 1 + s.backoff.Jitter*(rand.Float64()*2-1)
		select {
		case <-time.After(time.Duration(delay)):
			retries++
		case <-s.parentCtx.Done():
			return
		}
	}
}
