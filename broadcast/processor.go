package broadcast

import (
	"context"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastProcessor struct {
	broadcastID           uint64
	router                Router
	broadcastChan         chan Msg
	sendChan              chan Content
	ctx                   context.Context
	cancelFunc            context.CancelFunc
	started               time.Time
	ended                 time.Time
	cancellationCtx       context.Context
	cancellationCtxCancel context.CancelFunc // should only be called by the shard
	sentCancellation      bool

	executionOrder map[string]int
	orderIndex     int
	outOfOrderMsgs map[string][]Content
}

type metadata struct {
	OriginAddr        string
	OriginMethod      string
	Sent              bool
	ResponseMsg       protoreflect.ProtoMessage
	ResponseErr       error
	SendFn            func(protoreflect.ProtoMessage, error) error
	IsBroadcastClient bool
}

func (p *BroadcastProcessor) handle(msg Content) {
	p.broadcastID = msg.BroadcastID
	// defining metadata and methods here to prevent allocation on the heap
	metadata := &metadata{
		OriginAddr:        msg.OriginAddr,
		OriginMethod:      msg.OriginMethod,
		IsBroadcastClient: msg.IsBroadcastClient,
		SendFn:            msg.SendFn,
		Sent:              false,
	}
	methods := make([]string, 0, 3)
	p.initOrder()
	// connect to client immediately to potentially save some time
	go p.router.Connect(metadata.OriginAddr)
	defer func() {
		p.ended = time.Now()
		p.cancelFunc()
		p.cancellationCtxCancel()
		// mark allocations ready for GC
		p.outOfOrderMsgs = nil
	}()
	for {
		select {
		case <-p.ctx.Done():
			return
		case bMsg := <-p.broadcastChan:
			if p.broadcastID != bMsg.BroadcastID {
				continue
			}
			if bMsg.Broadcast {
				if p.handleBroadcast(bMsg, methods, metadata) {
					// methods keeps track of which methods has been broadcasted to.
					// This prevents duplicate broadcasts.
					methods = append(methods, bMsg.Method)
				}
			} else {
				if p.handleReply(bMsg, metadata) {
					// request is done if a reply is sent to the client.
					return
				}
			}
		case new := <-p.sendChan:
			if p.broadcastID != new.BroadcastID {
				new.ReceiveChan <- shardResponse{
					err: BroadcastIDErr{},
				}
				continue
			}
			if new.IsCancellation {
				// the cancellation implementation is just an
				// empty function and does not need the ctx or
				// broadcastChan.
				new.ReceiveChan <- shardResponse{
					err: nil,
				}
				continue
			}
			metadata.update(new)
			// this only pertains to requests where the server has a
			// direct connection to the client, e.g. QuorumCall.
			if metadata.Sent && !metadata.isBroadcastCall() {
				// we must return an error to prevent executing the implementation func.
				// This is because the server has finished the request and tried to reply
				// to the client previously. The msg we have just received is from the client,
				// meaning we can finally return the cached response.
				err := metadata.send(metadata.ResponseMsg, metadata.ResponseErr)
				if err == nil {
					err = AlreadyProcessedErr{}
				}
				new.ReceiveChan <- shardResponse{
					err: err,
				}
				return
			}
			if !p.isInOrder(new.CurrentMethod) {
				// save the message and execute it later
				p.addToOutOfOrder(new)
				new.ReceiveChan <- shardResponse{
					err: OutOfOrderErr{},
				}
			}
			new.ReceiveChan <- shardResponse{
				err:              nil,
				reqCtx:           p.cancellationCtx,
				enqueueBroadcast: p.enqueueBroadcast,
			}
		}
	}
}

func (p *BroadcastProcessor) handleBroadcast(bMsg Msg, methods []string, metadata *metadata) bool {
	// check if msg has already been broadcasted for this method
	//if alreadyBroadcasted(p.metadata.Methods, bMsg.Method) {
	if alreadyBroadcasted(methods, bMsg.Method) {
		return false
	}
	p.router.Send(p.broadcastID, metadata.OriginAddr, metadata.OriginMethod, bMsg.Msg)

	p.updateOrder(bMsg.Method)
	p.dispatchOutOfOrderMsgs()
	return true
}

func (p *BroadcastProcessor) handleReply(bMsg Msg, metadata *metadata) bool {
	// BroadcastCall if origin addr is non-empty.
	if metadata.isBroadcastCall() {
		go p.router.Send(p.broadcastID, metadata.OriginAddr, metadata.OriginMethod, bMsg.Reply)
		// the request is done becuase we have sent a reply to the client
		return true
	}
	// QuorumCall if origin addr is empty.

	// this sends a reply back to the client only if the client has
	// connected to the server. Otherwise, an error will be returned.
	// We thus need to cache the msg until the client has connected to
	// the server.
	err := metadata.send(bMsg.Reply.Response, bMsg.Reply.Err)
	if err != nil {
		// add response if not already done
		if metadata.ResponseMsg == nil {
			metadata.ResponseMsg = bMsg.Reply.Response
			metadata.ResponseErr = bMsg.Reply.Err
			metadata.Sent = true
		}
		// the request is not done yet because we have not replied to
		// the client.
		return false
	}
	// the request is done becuase we have sent a reply to the client
	return true
}

func (m *metadata) update(new Content) {
	if m.OriginAddr == "" && new.OriginAddr != "" {
		m.OriginAddr = new.OriginAddr
	}
	if m.OriginMethod == "" && new.OriginMethod != "" {
		m.OriginMethod = new.OriginMethod
	}
	if m.SendFn == nil && new.SendFn != nil {
		m.SendFn = new.SendFn
		m.IsBroadcastClient = new.IsBroadcastClient
	}
}

func (m *metadata) isBroadcastCall() bool {
	return m.OriginAddr != ""
}

func (m *metadata) send(resp protoreflect.ProtoMessage, err error) error {
	if !m.hasReceivedClientRequest() {
		return MissingClientReqErr{}
	}
	// error is intentionally ignored. We have not setup retry logic for failed
	// deliveries to clients. Responding with nil will stop the broadcast request
	// which is needed to prevent many stale goroutines.
	_ = m.SendFn(resp, err)
	return nil
}

func (m *metadata) hasReceivedClientRequest() bool {
	return m.IsBroadcastClient && m.SendFn != nil
}

//func alreadyBroadcasted(methods []string, method string) bool {
//for _, m := range methods {
//if m == method {
//return true
//}
//}
//return false
//}

//func (c *Content) isBroadcastCall() bool {
//return c.OriginAddr != ""
//}

//func (c *Content) hasReceivedClientRequest() bool {
//return c.IsBroadcastClient && c.SendFn != nil
//}

func (r *BroadcastProcessor) initOrder() {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	r.outOfOrderMsgs = make(map[string][]Content)
}

func (r *BroadcastProcessor) isInOrder(method string) bool {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return true
	}
	order, ok := r.executionOrder[method]
	// accept all methods without a specified order
	if !ok {
		return true
	}
	// the first method should always be allowed to be executed
	if r.executionOrder[method] <= 0 {
		return true
	}
	return order <= r.orderIndex
}

func (r *BroadcastProcessor) addToOutOfOrder(msg Content) {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	var (
		msgs []Content
		ok   bool
	)
	if msgs, ok = r.outOfOrderMsgs[msg.CurrentMethod]; ok {
		msgs = append(msgs, msg)
	} else {
		msgs = []Content{msg}
	}
	r.outOfOrderMsgs[msg.CurrentMethod] = msgs
}

func (r *BroadcastProcessor) updateOrder(method string) {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	order, ok := r.executionOrder[method]
	// do nothing for methods without specified order
	if !ok {
		return
	}
	if order > r.orderIndex {
		r.orderIndex = order
	}
}

func (r *BroadcastProcessor) dispatchOutOfOrderMsgs() {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	// return early if there are no cached msgs
	if len(r.outOfOrderMsgs) <= 0 {
		return
	}
	handledMethods := make([]string, 0, len(r.outOfOrderMsgs))
	for method, msgs := range r.outOfOrderMsgs {
		order, ok := r.executionOrder[method]
		if !ok {
			// this should not be possible unless the execution order
			// is changed during operation, which is prohibited.
			panic("how did you get here?")
		}
		if order <= r.orderIndex {
			for _, msg := range msgs {
				msg.Run(r.cancellationCtx, r.enqueueBroadcast)
			}
			handledMethods = append(handledMethods, method)
		}
	}
	// cleanup after dispatching the cached messages
	for _, m := range handledMethods {
		delete(r.outOfOrderMsgs, m)
	}
}

// this method is used to enqueue messages onto the broadcast channel
// of a broadcast request. The messages enqueued are then transmitted
// to the other servers or the client depending on the type of message.
// Currently there are three types:
// - BroadcastMsg
// - ClientReply
// - Cancellation
func (req *BroadcastProcessor) enqueueBroadcast(msg Msg) error {
	select {
	case req.broadcastChan <- msg:
		return nil
	case <-req.ctx.Done():
		return AlreadyProcessedErr{}
	}
}
