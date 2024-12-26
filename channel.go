package gorums

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relab/gorums/logging"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var streamDownErr = status.Error(codes.Unavailable, "stream is down")

type request struct {
	ctx       context.Context
	msg       *Message
	opts      callOptions
	numFailed int
}

// waitForSend returns true if the WithNoSendWaiting call option is not set.
func (req request) waitForSend() bool {
	return req.opts.callType != nil && !req.opts.noSendWaiting
}

// relatedToBroadcast returns true if the request is related to a broadcast request.
func (req request) relatedToBroadcast() bool {
	if req.msg.Metadata == nil {
		return false
	}
	if req.msg.Metadata.BroadcastMsg == nil {
		return false
	}
	if req.msg.Metadata.BroadcastMsg.BroadcastID <= 0 {
		return false
	}
	return true
}

// getLogArgs returns the args given to the structured logger.
func (req request) getLogArgs(c *channel) []slog.Attr {
	// no need to do processing if logging is not enabled
	if c.logger == nil {
		return nil
	}
	args := []slog.Attr{logging.MsgID(req.msg.Metadata.MessageID), logging.Method(req.msg.Metadata.Method), logging.NumFailed(req.numFailed), logging.MaxRetries(c.maxSendRetries)}
	if req.relatedToBroadcast() {
		args = append(args, logging.BroadcastID(req.msg.Metadata.BroadcastMsg.BroadcastID))
	}
	return args
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
	dialMut         sync.RWMutex
	streamBroken    atomicFlag
	connEstablished atomicFlag
	parentCtx       context.Context
	streamCtx       context.Context
	cancelStream    context.CancelFunc
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex
	maxSendRetries  int // number of times we try to resend a failed msg
	maxConnRetries  int // number of times we try to reconnect to a node
	logger          *slog.Logger
}

// newChannel creates a new channel for the given node and starts the sending goroutine.
//
// Note that we start the sending goroutine even though the
// connection has not yet been established. This is to prevent
// deadlock when invoking a call type, as the goroutine will
// block on the sendQ until a connection has been established.
func newChannel(n *RawNode) *channel {
	var logger *slog.Logger
	if n.mgr.logger != nil {
		logger = n.mgr.logger.With(slog.Uint64("nodeID", uint64(n.ID())), slog.String("nodeAddr", n.Address()))
	}
	c := &channel{
		sendQ:           make(chan request, n.mgr.opts.sendBuffer),
		backoffCfg:      n.mgr.opts.backoff,
		node:            n,
		latency:         -1 * time.Second,
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
		responseRouters: make(map[uint64]responseRouter),
		maxSendRetries:  n.mgr.opts.maxSendRetries,
		maxConnRetries:  n.mgr.opts.maxConnRetries,
		logger:          logger,
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
		c.log("channel: cancelling pending msg", streamDownErr, slog.LevelError, logging.MsgID(msgID))
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
		c.responseRouters[req.msg.Metadata.MessageID] = responseRouter{responseChan, streaming}
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed.
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
		return
	case c.sendQ <- req:
	}
}

func (c *channel) enqueueFast(req request, responseChan chan<- response, streaming bool) bool {
	if responseChan != nil {
		c.responseMut.Lock()
		c.responseRouters[req.msg.Metadata.MessageID] = responseRouter{responseChan, streaming}
		c.responseMut.Unlock()
	}
	// only enqueue the request on the sendQ if it is available and
	// the node is not closed.
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
	case c.sendQ <- req:
	default:
		return false
	}
	return true
}

func (c *channel) enqueueSlow(req request) {
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed.
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
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
		//
		// It is important to note that waitForSend() should NOT return true if a response
		// is expected from the node at the other end, e.g. when using RPCCall or QuorumCall.
		// CallOptions are not provided with the requests from these types and thus waitForSend()
		// returns false. This design should maybe be revised?
		if req.waitForSend() {
			// unblock the caller and clean up the responseRouter map
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
			// Both channels could be ready at the same time, so we must check 'done' again.
			select {
			case <-done:
				// false alarm
			default:
				// CANCELLING HERE CAN HAVE DESTRUCTIVE EFFECTS!
				// Imagine the client has sent several requests and is waiting
				// for a response on each individual request. Furthermore, let's
				// say the client has sent a message to two different handlers:
				// 		1. A handler that does a lot of work and thus long response times are expected.
				// 		2. A handler that is normally very fast.
				//
				// If the client is impatient and cancels a request sent to a handler in scenario 2,
				// then all requests sent to the handler in scenario 1 will also be cancelled because
				// the stream is taken down.

				// trigger reconnect
				//c.streamMut.Lock()
				//c.cancelStream()
				//c.streamMut.Unlock()
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
			err := c.connect()
			if err != nil {
				c.setLastErr(err)
				c.streamBroken.set()
			}
		}
		// retry if stream is broken
		if c.streamBroken.get() {
			go c.retryMsg(req, streamDownErr)
			continue
		}
		// else try to send message
		err := c.sendMsg(req)
		if err != nil {
			go c.retryMsg(req, err)
		} else {
			c.log("channel: successfully sent msg", nil, slog.LevelInfo, req.getLogArgs(c)...)
		}
	}
}

func (c *channel) receiver() {
	for {
		resp := newMessage(responseType)
		c.streamMut.RLock()
		var err error
		// the gorumsStream can be nil because this method
		// runs in a goroutine. If the stream goes down after
		// the streamMut is unlocked, then we can have a scenario
		// where the gorumsStream is set to nil in the reconnect
		// method by another goroutine.
		if c.gorumsStream != nil {
			err = c.gorumsStream.RecvMsg(resp)
		} else {
			err = streamDownErr
		}
		if err != nil {
			c.streamBroken.set()
			c.streamMut.RUnlock()
			c.setLastErr(err)
			// we only reach this point when the stream failed AFTER a message
			// was sent and we are waiting for a reply. We thus need to respond
			// with a stream is down error on all pending messages.
			c.cancelPendingMsgs()
			c.log("channel: lost connection", err, slog.LevelError, logging.Reconnect(true))
			// attempt to reconnect indefinitely until the node is closed.
			// This is necessary when streaming is enabled.
			c.reconnect(-1)
		} else {
			c.streamMut.RUnlock()
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			c.routeResponse(resp.Metadata.MessageID, response{nid: c.node.ID(), msg: resp.Message, err: err})
			if err != nil {
				c.log("channel: got response", err, slog.LevelError, logging.MsgID(resp.Metadata.MessageID), logging.Method(resp.Metadata.Method))
			} else {
				c.log("channel: got response", nil, slog.LevelInfo, logging.MsgID(resp.Metadata.MessageID), logging.Method(resp.Metadata.Method))
			}
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
		c.dialMut.Lock()
		defer c.dialMut.Unlock()
		// a connection has not yet been established; i.e.,
		// a previous dial attempt could have failed.
		// try dialing again.
		err := c.node.dial()
		if err != nil {
			return err
		}
		err = c.newNodeStream(c.node.conn)
		if err != nil {
			return err
		}
		// return early because streamBroken will be cleared if no error occurs.
		// also works a preemptive measure to a deadlock since dialMut.Unlock()
		// is deferred.
		return nil
	}
	// the node was previously connected but is now disconnected
	if c.streamBroken.get() {
		// try to reconnect only once.
		// Maybe add this as a user option?
		c.reconnect(1)
	}
	return nil
}

type retry float64

func (r retry) exceeds(maxRetries int) bool {
	return r >= retry(maxRetries) && maxRetries > 0
}

// reconnect tries to reconnect to the node using an exponential backoff strategy.
// maxRetries = -1 represents infinite retries.
func (c *channel) reconnect(maxRetries int) {
	select {
	case <-c.parentCtx.Done():
		// no need to try to reconnect if the node is closed.
		// make sure to cancel the previous ctx to prevent context leakage
		if c.cancelStream != nil {
			c.cancelStream()
		}
		return
	default:
	}
	backoffCfg := c.backoffCfg

	var retries retry
	for {
		var err error
		c.streamMut.Lock()
		// check if stream is already up
		if !c.streamBroken.get() {
			// do nothing because stream is up
			c.streamMut.Unlock()
			return
		}
		// make sure to cancel the previous ctx to prevent context leakage
		if c.cancelStream != nil {
			c.cancelStream()
		}
		c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
		c.gorumsStream, err = c.gorumsClient.NodeStream(c.streamCtx)
		if err == nil {
			c.log("channel: restored connection", nil, slog.LevelInfo)
			c.streamBroken.clear()
			c.streamMut.Unlock()
			return
		}
		c.log("channel: reconnection failed", err, slog.LevelWarn, logging.RetryNum(float64(retries)), logging.Reconnect(!(retries.exceeds(maxRetries) || retries.exceeds(c.maxConnRetries))))
		c.cancelStream()
		c.streamMut.Unlock()
		c.setLastErr(err)
		if retries.exceeds(maxRetries) || retries.exceeds(c.maxConnRetries) {
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

// This method should always be run in a goroutine. It will
// enqueue a msg if it has previously failed. The message will
// be dropped if it fails more than maxRetries or if the ctx
// is cancelled.
func (c *channel) retryMsg(req request, err error) {
	req.numFailed++
	c.log("channel: failed to send msg", err, slog.LevelError, req.getLogArgs(c)...)
	// c.maxRetries = -1, is the same as infinite retries.
	if req.numFailed > c.maxSendRetries && c.maxSendRetries != -1 {
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("max retries exceeded. err=%e", err)})
		return
	}
	delay := float64(c.backoffCfg.BaseDelay)
	//delay := float64(10 * time.Millisecond)
	max := float64(c.backoffCfg.MaxDelay)
	for r := req.numFailed; delay < max && r > 0; r-- {
		delay *= c.backoffCfg.Multiplier
	}
	delay = math.Min(delay, max)
	delay *= 1 + c.backoffCfg.Jitter*(rand.Float64()*2-1)
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
		return
	case <-req.ctx.Done():
		c.routeResponse(req.msg.Metadata.MessageID, response{nid: c.node.ID(), err: fmt.Errorf("context cancelled")})
		return
	case <-time.After(time.Duration(delay)):
		// enqueue the request again
	}
	c.enqueueSlow(req)
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

func (c *channel) log(msg string, err error, level slog.Level, args ...slog.Attr) {
	if c.logger != nil {
		args = append(args, logging.Err(err), logging.Type("channel"))
		c.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}

type atomicFlag struct {
	flag int32
}

func (f *atomicFlag) set()      { atomic.StoreInt32(&f.flag, 1) }
func (f *atomicFlag) get() bool { return atomic.LoadInt32(&f.flag) == 1 }
func (f *atomicFlag) clear()    { atomic.StoreInt32(&f.flag, 0) }
