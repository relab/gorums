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
	connEstablished atomicFlag
	parentCtx       context.Context
	streamCtx       context.Context
	cancelStream    context.CancelFunc
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex
}

func newChannel(n *RawNode) *channel {
	return &channel{
		sendQ:           make(chan request, n.mgr.opts.sendBuffer),
		backoffCfg:      n.mgr.opts.backoff,
		node:            n,
		latency:         -1 * time.Second,
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
		responseRouters: make(map[uint64]responseRouter),
	}
}

func (c *channel) connect(ctx context.Context, conn *grpc.ClientConn) error {
	// the parentCtx governs the channel and is used to properly
	// shut it down
	c.parentCtx = ctx
	// it is important to start the goroutine regardless of a
	// successful connection to prevent a deadlock when
	// invoking one of the call types. The method provides
	// a listener on the sendQ and contains the retry logic
	go c.sendMsgs()
	return c.tryConnect(conn)
}

// creating a stream could fail even though conn != nil due to
// the non-blocking dial. Hence, we need to try to connect to
// the node before starting the receiving goroutine
func (c *channel) tryConnect(conn *grpc.ClientConn) error {
	if conn == nil {
		// no need to proceed if dial setup failed
		return fmt.Errorf("connection is nil")
	}
	c.streamMut.Lock()
	var err error
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsClient = ordering.NewGorumsClient(conn)
	c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
	c.streamMut.Unlock()
	if err != nil {
		return err
	}
	c.streamBroken.clear()
	// safe guard because creating more than one recvMsgs goroutine is problematic
	if !c.connEstablished.get() {
		// connEstablished indicates whether recvMsgs have been started or not and if
		// the dial was successful. streamBroken only reports the status of the stream.
		c.connEstablished.set()
		go c.recvMsgs()
	}
	return nil
}

func (c *channel) cancelPendingMsgs() {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	for msgID, router := range c.responseRouters {
		err := status.Errorf(codes.Unavailable, "stream is down")
		router.c <- response{nid: c.node.ID(), msg: nil, err: err}
		// delete the router if we are only expecting a single message
		if !router.streaming {
			delete(c.responseRouters, msgID)
		}
	}
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
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
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
			// streamBroken will be set if the reconnection fails
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
			// The only time we reach this point is when the
			// stream goes down AFTER a message has been sent and the node
			// is waiting for a reply. We thus need to respond with a stream
			// is down error on all pending messages.
			c.cancelPendingMsgs()
			// attempt to reconnect. It will try to reconnect indefinitely
			// or until the node is closed. This is necessary when streaming
			// is enabled.
			c.reconnect(-1)
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
	if !c.connEstablished.get() {
		// a connection has never been established; i.e.,
		// a previous dial attempt could have failed.
		// we need to make sure the connection is up.
		err := c.node.dial()
		if err != nil {
			c.streamBroken.set()
			return
		}
		// try to create a stream. Should NOT be
		// run if a connection has previously
		// been established because it will start
		// a recvMsgs goroutine. Otherwise, we
		// could suffer from leaking goroutines.
		// a guardclause has been added in the
		// method to prevent this.
		err = c.tryConnect(c.node.conn)
		if err != nil {
			c.streamBroken.set()
			return
		}
	}
	// the node has previously been connected
	// but is now disconnected
	if c.streamBroken.get() {
		// try to reconnect only once.
		// Maybe add this as a user option?
		c.reconnect(1)
	}
}

// reconnect tries to reconnect to the node using an exponential backoff strategy.
// maxRetries = -1 represents infinite retries.
func (c *channel) reconnect(maxRetries float64) {
	backoffCfg := c.backoffCfg

	var retries float64
	for {
		var err error
		c.streamMut.Lock()
		// check if stream is already up
		if !c.streamBroken.get() {
			// do nothing because stream is up
			c.streamMut.Unlock()
			return
		}
		c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
		c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
		if err == nil {
			c.streamBroken.clear()
			c.streamMut.Unlock()
			return
		}
		c.cancelStream()
		c.streamMut.Unlock()
		c.setLastErr(err)
		if retries >= maxRetries && maxRetries > 0 {
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

// isConnected returns true if the channel has an active connection to the node.
func (c *channel) isConnected() bool {
	// streamBroken.get() is initially false and NodeStream could be down
	// even though node.conn is not nil. Hence, we need connEstablished
	// to make sure a proper connection has been made.
	return c.connEstablished.get() && !c.streamBroken.get()
}

type atomicFlag struct {
	flag int32
}

func (f *atomicFlag) set()      { atomic.StoreInt32(&f.flag, 1) }
func (f *atomicFlag) get() bool { return atomic.LoadInt32(&f.flag) == 1 }
func (f *atomicFlag) clear()    { atomic.StoreInt32(&f.flag, 0) }
