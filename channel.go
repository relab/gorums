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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type request struct {
	ctx  context.Context
	msg  *Message
	opts callOptions
}

type response struct {
	nid uint32
	msg protoreflect.ProtoMessage
	err error
}

type receiveQueue struct {
	msgID    uint64
	recvQ    map[uint64]chan *response
	recvQMut sync.RWMutex
}

func newReceiveQueue() *receiveQueue {
	return &receiveQueue{
		recvQ: make(map[uint64]chan *response),
	}
}

// newCall returns unique metadata for a method call.
func (m *receiveQueue) newCall(method string) (md *ordering.Metadata) {
	msgID := atomic.AddUint64(&m.msgID, 1)
	return &ordering.Metadata{
		MessageID: msgID,
		Method:    method,
	}
}

// newReply returns a channel for receiving replies
// and a done function to be called for clean up.
func (m *receiveQueue) newReply(md *ordering.Metadata, maxReplies int) (replyChan chan *response, done func()) {
	replyChan = make(chan *response, maxReplies)
	m.recvQMut.Lock()
	m.recvQ[md.MessageID] = replyChan
	m.recvQMut.Unlock()
	done = func() {
		m.recvQMut.Lock()
		delete(m.recvQ, md.MessageID)
		m.recvQMut.Unlock()
	}
	return
}

func (m *receiveQueue) putResult(id uint64, result *response) {
	m.recvQMut.RLock()
	c, ok := m.recvQ[id]
	m.recvQMut.RUnlock()
	if ok {
		c <- result
	}
}

type channel struct {
	sendQ        chan request
	node         *Node // needed for ID and setLastError
	rand         *rand.Rand
	gorumsClient ordering.GorumsClient
	gorumsStream ordering.Gorums_NodeStreamClient
	streamMut    sync.RWMutex
	streamBroken bool
	parentCtx    context.Context
	streamCtx    context.Context
	cancelStream context.CancelFunc
}

func newChannel(n *Node) *channel {
	return &channel{
		sendQ: make(chan request, n.opts.sendBuffer),
		node:  n,
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *channel) connect(ctx context.Context, conn *grpc.ClientConn) error {
	var err error
	c.parentCtx = ctx
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsClient = ordering.NewGorumsClient(conn)
	c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
	if err != nil {
		return err
	}
	go c.sendMsgs()
	go c.recvMsgs()
	return nil
}

func (c *channel) sendMsg(req request) (err error) {
	// unblock the waiting caller unless noSendWaiting is enabled
	defer func() {
		if req.opts.callType == E_Multicast || req.opts.callType == E_Unicast && !req.opts.noSendWaiting {
			c.node.putResult(req.msg.Metadata.MessageID, &response{})
		}
	}()

	// don't send if context is already cancelled.
	if req.ctx.Err() != nil {
		return req.ctx.Err()
	}

	c.streamMut.RLock()
	defer c.streamMut.RUnlock()

	done := make(chan struct{}, 1)

	// wait for either the message to be sent, or the request context being cancelled.
	// if the request context was cancelled, then we most likely have a blocked stream.
	go func() {
		select {
		case <-done:
		case <-req.ctx.Done():
			c.cancelStream()
		}
	}()

	err = c.gorumsStream.SendMsg(req.msg)
	if err != nil {
		c.node.setLastErr(err)
		c.streamBroken = true
	}
	done <- struct{}{}

	return err
}

func (c *channel) sendMsgs() {
	var req request
	for {
		select {
		case <-c.parentCtx.Done():
			return
		case req = <-c.sendQ:
		}
		// return error if stream is broken
		if c.streamBroken {
			err := status.Errorf(codes.Unavailable, "stream is down")
			c.node.putResult(req.msg.Metadata.MessageID, &response{nid: c.node.ID(), msg: nil, err: err})
			continue
		}
		// else try to send message
		err := c.sendMsg(req)
		if err != nil {
			// return the error
			c.node.putResult(req.msg.Metadata.MessageID, &response{nid: c.node.ID(), msg: nil, err: err})
		}
	}
}

func (c *channel) recvMsgs() {
	for {
		resp := newMessage(responseType)
		c.streamMut.RLock()
		err := c.gorumsStream.RecvMsg(resp)
		if err != nil {
			c.streamBroken = true
			c.streamMut.RUnlock()
			c.node.setLastErr(err)
			// attempt to reconnect
			c.reconnect()
		} else {
			c.streamMut.RUnlock()
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			c.node.putResult(resp.Metadata.MessageID, &response{nid: c.node.ID(), msg: resp.Message, err: err})
		}

		select {
		case <-c.parentCtx.Done():
			return
		default:
		}
	}
}

func (c *channel) reconnect() {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	backoffCfg := c.node.opts.backoff

	var retries float64
	for {
		var err error

		c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
		c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
		if err == nil {
			c.streamBroken = false
			return
		}
		c.cancelStream()
		c.node.setLastErr(err)
		delay := float64(backoffCfg.BaseDelay)
		max := float64(backoffCfg.MaxDelay)
		for r := retries; delay < max && r > 0; r-- {
			delay *= backoffCfg.Multiplier
		}
		delay = math.Min(delay, max)
		delay *= 1 + backoffCfg.Jitter*(rand.Float64()*2-1)
		select {
		case <-time.After(time.Duration(delay)):
			retries++
		case <-c.parentCtx.Done():
			return
		}
	}
}
