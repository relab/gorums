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

type channel struct {
	sendQ          chan request
	node           *Node // needed for ID and setLastError
	rand           *rand.Rand
	gorumsClient   ordering.GorumsClient
	gorumsStream   ordering.Gorums_NodeStreamClient
	streamMut      sync.RWMutex
	streamBroken   atomicFlag
	parentCtx      context.Context
	streamCtx      context.Context
	cancelStream   context.CancelFunc
	responseRouter map[uint64]chan<- response
	responseMut    sync.Mutex
}

func newChannel(n *Node) *channel {
	return &channel{
		sendQ:          make(chan request, n.mgr.opts.sendBuffer),
		node:           n,
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		responseRouter: make(map[uint64]chan<- response),
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

func (c *channel) routeResponse(msgID uint64, resp response) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	if ch, ok := c.responseRouter[msgID]; ok {
		ch <- resp
		delete(c.responseRouter, msgID)
	}
}

func (c *channel) enqueue(req request, responseChan chan<- response) {
	if responseChan != nil {
		c.responseMut.Lock()
		c.responseRouter[req.msg.Metadata.MessageID] = responseChan
		c.responseMut.Unlock()
	}
	c.sendQ <- req
}

func (c *channel) sendMsg(req request) (err error) {
	// unblock the waiting caller unless noSendWaiting is enabled
	defer func() {
		if req.opts.callType == E_Multicast || req.opts.callType == E_Unicast && !req.opts.noSendWaiting {
			c.routeResponse(req.msg.Metadata.MessageID, response{})
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
		c.streamBroken.set()
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
		if c.streamBroken.get() {
			err := status.Errorf(codes.Unavailable, "stream is down")
			c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), msg: nil, err: err})
			continue
		}
		// else try to send message
		err := c.sendMsg(req)
		if err != nil {
			// return the error
			c.node.channel.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), msg: nil, err: err})
		}
	}
}

func (c *channel) recvMsgs() {
	for {
		resp := newMessage(responseType)
		c.streamMut.RLock()
		err := c.gorumsStream.RecvMsg(resp)
		if err != nil {
			c.streamBroken.set()
			c.streamMut.RUnlock()
			c.node.setLastErr(err)
			// attempt to reconnect
			c.reconnect()
		} else {
			c.streamMut.RUnlock()
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			c.routeResponse(resp.Metadata.MessageID, response{nid: c.node.ID(), msg: resp.Message, err: err})
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
	backoffCfg := c.node.mgr.opts.backoff

	var retries float64
	for {
		var err error

		c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
		c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
		if err == nil {
			c.streamBroken.clear()
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

type atomicFlag struct {
	flag int32
}

func (f *atomicFlag) set()      { atomic.StoreInt32(&f.flag, 1) }
func (f *atomicFlag) get() bool { return atomic.LoadInt32(&f.flag) == 1 }
func (f *atomicFlag) clear()    { atomic.StoreInt32(&f.flag, 0) }
