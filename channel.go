package gorums

import (
	"context"
	"fmt"
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

type responseRouter struct {
	c         chan<- response
	streaming bool
}

type channel struct {
	sendQ           chan request
	node            *RawNode
	mu              sync.Mutex
	lastError       error
	latency         time.Duration
	backoffCfg      backoff.Config
	rand            *rand.Rand
	gorumsClient    ordering.GorumsClient
	gorumsStream    ordering.Gorums_NodeStreamClient
	streamMut       sync.RWMutex
	streamBroken    atomicFlag
	parentCtx       context.Context
	streamCtx       context.Context
	cancelStream    context.CancelFunc
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex
	connEstablished bool
}

func newChannel(n *RawNode) *channel {
	c := &channel{
		sendQ:           make(chan request, n.mgr.opts.sendBuffer),
		backoffCfg:      n.mgr.opts.backoff,
		node:            n,
		latency:         -1 * time.Second,
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
		responseRouters: make(map[uint64]responseRouter),
		connEstablished: false,
	}
	return c
}

func (c *channel) connect(ctx context.Context, conn *grpc.ClientConn) error {
	c.parentCtx = ctx
	go c.sendMsgs()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return c.tryConnect(conn)
}

func (c *channel) tryConnect(conn *grpc.ClientConn) error {
	var err error
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsClient = ordering.NewGorumsClient(conn)
	c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
	if err != nil {
		return err
	}
	c.connEstablished = true
	go c.recvMsgs()
	return nil
}

func (c *channel) routeResponse(msgID uint64, resp response) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	if router, ok := c.responseRouters[msgID]; ok {
		router.c <- resp
		// delete the router if we are only expecting a single message
		if !router.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

func (c *channel) enqueue(req request, responseChan chan<- response, streaming bool) {
	if responseChan != nil {
		c.responseMut.Lock()
		c.responseRouters[req.msg.Metadata.MessageID] = responseRouter{responseChan, streaming}
		c.responseMut.Unlock()
	}
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), msg: nil, err: fmt.Errorf("channel closed")})
		return
	case c.sendQ <- req:
	}
}

func (c *channel) deleteRouter(msgID uint64) {
	delete(c.responseRouters, msgID)
}

func (c *channel) sendMsg(req request) (err error) {
	// unblock the waiting caller unless noSendWaiting is enabled
	defer func() {
		if req.opts.callType == E_Multicast || req.opts.callType == E_Broadcast || req.opts.callType == E_Unicast && !req.opts.noSendWaiting {
			c.routeResponse(req.msg.Metadata.MessageID, response{})
		}
	}()

	// don't send if context is already cancelled.
	if req.ctx.Err() != nil {
		return req.ctx.Err()
	}

	c.streamMut.RLock()
	defer c.streamMut.RUnlock()

	done := make(chan struct{})

	// This goroutine waits for either 'done' to be closed, or the request context to be cancelled.
	// If the request context was cancelled, we have two possibilities:
	// The stream could be blocked, or the caller could be impatient.
	// We cannot know which is the case, but it seems wiser to cancel the stream as a precaution,
	// because reconnection is quite fast and cheap.
	go func() {
		select {
		case <-done:
			// all is good
		case <-req.ctx.Done():
			// Both channels could be ready at the same time, so we should check 'done' again.
			select {
			case <-done:
				// false alarm
			default:
				// cause reconnect
				c.cancelStream()
			}
		}
	}()

	err = c.gorumsStream.SendMsg(req.msg)
	if err != nil {
		c.setLastErr(err)
		c.streamBroken.set()
	}

	close(done)

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
		// try to connect to the node if previous attempts
		// have failed or if the node has disconnected
		if !c.isConnected() {
			// streamBroken will be set if the connection fails
			c.tryReconnect()
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
			c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), msg: nil, err: err})
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
			c.setLastErr(err)
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

func (c *channel) tryReconnect() {
	// a connection has never been established
	if !c.connEstablished {
		err := c.node.dial()
		if err != nil {
			c.streamBroken.set()
			return
		}
		err = c.tryConnect(c.node.conn)
		if err != nil {
			c.streamBroken.set()
			return
		}
	}
	// the node has previously been connected
	// but is now disconnected
	if c.streamBroken.get() {
		// try to reconnect only once
		c.reconnect(1)
	}
}

func (c *channel) reconnect(maxRetries ...int) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	backoffCfg := c.backoffCfg

	var maxretries float64 = -1
	if len(maxRetries) > 0 {
		maxretries = float64(maxRetries[0])
	}
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
		c.setLastErr(err)
		if retries >= maxretries && maxretries > 0 {
			c.streamBroken.set()
			return
		}
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

func (c *channel) setLastErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

// lastErr returns the last error encountered (if any) when using this channel.
func (c *channel) lastErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// channelLatency returns the latency between the client and this channel.
func (c *channel) channelLatency() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latency
}

func (c *channel) isConnected() bool {
	return c.connEstablished && !c.streamBroken.get()
}

type atomicFlag struct {
	flag int32
}

func (f *atomicFlag) set()      { atomic.StoreInt32(&f.flag, 1) }
func (f *atomicFlag) get() bool { return atomic.LoadInt32(&f.flag) == 1 }
func (f *atomicFlag) clear()    { atomic.StoreInt32(&f.flag, 0) }
