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
	"google.golang.org/protobuf/proto"
)

var streamDownErr = status.Error(codes.Unavailable, "stream is down")

type request struct {
	ctx  context.Context
	msg  *Message
	opts callOptions
}

// waitForSend returns true if the WithNoSendWaiting call option is not set.
func (req request) waitForSend() bool {
	return req.opts.callType != nil && !req.opts.noSendWaiting
}

type response struct {
	nid uint32
	msg proto.Message
	err error
}

type responseRouter struct {
	c         chan<- response
	streaming bool
}

type channel struct {
	sendQ           chan request
	node            *Node
	mu              sync.Mutex
	lastError       error
	latency         time.Duration
	backoffCfg      backoff.Config
	rand            *rand.Rand
	gorumsClient    ordering.GorumsClient
	gorumsStream    grpc.BidiStreamingClient[ordering.Metadata, ordering.Metadata]
	streamMut       sync.RWMutex
	streamBroken    atomicFlag
	connEstablished atomicFlag
	parentCtx       context.Context
	streamCtx       context.Context
	cancelStream    context.CancelFunc
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex
}

// newChannel creates a new channel for the given node and starts the sending goroutine.
//
// Note that we start the sending goroutine even though the
// connection has not yet been established. This is to prevent
// deadlock when invoking a call type, as the goroutine will
// block on the sendQ until a connection has been established.
func newChannel(n *Node) *channel {
	c := &channel{
		sendQ:           make(chan request, n.mgr.opts.sendBuffer),
		backoffCfg:      n.mgr.opts.backoff,
		node:            n,
		latency:         -1 * time.Second,
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
		responseRouters: make(map[uint64]responseRouter),
	}
	// parentCtx controls the channel and is used to shut it down
	c.parentCtx = n.newContext()
	go c.sender()
	return c
}

// newNodeStream creates a stream and starts the receiving goroutine.
//
// Note that the stream could fail even though conn != nil due
// to the non-blocking dial. Hence, we need to try to connect
// to the node before starting the receiving goroutine.
func (c *channel) newNodeStream(conn *grpc.ClientConn) error {
	if conn == nil {
		// no need to proceed if dial failed
		return fmt.Errorf("connection is nil")
	}
	c.streamMut.Lock()
	var err error
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsClient = ordering.NewGorumsClient(conn)
	c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
	c.streamMut.Unlock()
	if err != nil {
		fmt.Println("oops " + err.Error())
		return err
	}
	c.streamBroken.clear()
	// guard against creating multiple receiver goroutines
	if !c.connEstablished.get() {
		// connEstablished indicates dial was successful
		// and that receiver have started
		c.connEstablished.set()
		go c.receiver()
	}
	return nil
}

func (c *channel) cancelPendingMsgs() {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	for msgID, router := range c.responseRouters {
		router.c <- response{nid: c.node.ID(), err: streamDownErr}
		// delete the router if we are only expecting a single reply message
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
		// delete the router if we are only expecting a single reply message
		if !router.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

func (c *channel) enqueue(req request, responseChan chan<- response, streaming bool) {
	if responseChan != nil {
		c.responseMut.Lock()
		c.responseRouters[req.msg.Metadata.GetMessageID()] = responseRouter{responseChan, streaming}
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
		return
	case c.sendQ <- req:
	}
}

func (c *channel) deleteRouter(msgID uint64) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	delete(c.responseRouters, msgID)
}

func (c *channel) sendMsg(req request) (err error) {
	defer func() {
		// While the default is to block the caller until the message has been sent, we
		// can provide the WithNoSendWaiting call option to more quickly unblock the caller.
		// Hence, after sending, we unblock the waiting caller if the call option is not set;
		// that is, waitForSend is true. Conversely, if the call option is set, the call type
		// will not block on the response channel, and the "receiver" goroutine below will
		// eventually clean up the responseRouter map by calling routeResponse.
		if req.waitForSend() {
			// unblock the caller and clean up the responseRouter map
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{})
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
			// Both channels could be ready at the same time, so we must check 'done' again.
			select {
			case <-done:
				// false alarm
			default:
				// trigger reconnect
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

func (c *channel) sender() {
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
			c.connect()
		}
		// return error if stream is broken
		if c.streamBroken.get() {
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: streamDownErr})
			continue
		}
		// else try to send message
		err := c.sendMsg(req)
		if err != nil {
			// return the error
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: err})
		}
	}
}

func (c *channel) receiver() {
	for {
		resp := newMessage(responseType)
		c.streamMut.RLock()
		err := c.gorumsStream.RecvMsg(resp)
		if err != nil {
			c.streamBroken.set()
			c.streamMut.RUnlock()
			c.setLastErr(err)
			// we only reach this point when the stream failed AFTER a message
			// was sent and we are waiting for a reply. We thus need to respond
			// with a stream is down error on all pending messages.
			c.cancelPendingMsgs()
			// attempt to reconnect indefinitely until the node is closed.
			// This is necessary when streaming is enabled.
			c.reconnect(-1)
		} else {
			c.streamMut.RUnlock()
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			c.routeResponse(resp.Metadata.GetMessageID(), response{nid: c.node.ID(), msg: resp.Message, err: err})
		}

		select {
		case <-c.parentCtx.Done():
			return
		default:
		}
	}
}

func (c *channel) connect() error {
	if !c.connEstablished.get() {
		// a connection has not yet been established; i.e.,
		// a previous dial attempt could have failed.
		// try dialing again.
		err := c.node.dial()
		if err != nil {
			c.streamBroken.set()
			return err
		}
		err = c.newNodeStream(c.node.conn)
		if err != nil {
			c.streamBroken.set()
			return err
		}
	}
	// the node was previously connected but is now disconnected
	if c.streamBroken.get() {
		// try to reconnect only once.
		// Maybe add this as a user option?
		c.reconnect(1)
	}
	return nil
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
		maxDelay := float64(backoffCfg.MaxDelay)
		for r := retries; delay < maxDelay && r > 0; r-- {
			delay *= backoffCfg.Multiplier
		}
		delay = math.Min(delay, maxDelay)
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
