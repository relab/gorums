package broadcast

import (
	"context"
	"log/slog"
	"time"

	"github.com/relab/gorums/logging"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastProcessor struct {
	broadcastID   uint64
	router        Router
	broadcastChan chan *Msg
	sendChan      chan *Content
	ctx           context.Context
	cancelFunc    context.CancelFunc
	started       time.Time
	ended         time.Time
	logger        *slog.Logger

	cancellationCtx       context.Context
	cancellationCtxCancel context.CancelFunc

	// ordering
	executionOrder map[string]int
	orderIndex     int
	outOfOrderMsgs map[string][]*Content
}

type metadata struct {
	OriginAddr           string
	OriginMethod         string
	Sent                 bool
	ResponseMsg          protoreflect.ProtoMessage
	ResponseErr          error
	SendFn               func(protoreflect.ProtoMessage, error) error
	IsBroadcastClient    bool
	SentCancellation     bool
	HasReceivedClientReq bool
}

func (p *BroadcastProcessor) handle(msg *Content) {
	p.log("processor: started", nil, logging.Started(p.started))
	p.broadcastID = msg.BroadcastID
	// defining metadata and methods here to prevent allocation on the heap
	metadata := &metadata{
		OriginAddr:        msg.OriginAddr,
		OriginMethod:      msg.OriginMethod,
		IsBroadcastClient: msg.IsBroadcastClient,
		SendFn:            msg.SendFn,
		Sent:              false,
		SentCancellation:  false,
	}
	methods := make([]string, 0, 3)
	p.initOrder()
	// connect to client immediately to potentially save some time
	go p.router.Connect(metadata.OriginAddr)
	if msg.ReceiveChan != nil {
		if !p.isInOrder(msg.CurrentMethod) {
			// save the message and execute it later
			p.addToOutOfOrder(msg)
			msg.ReceiveChan <- shardResponse{
				err: OutOfOrderErr{},
			}
			p.log("msg: out of order", OutOfOrderErr{}, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
		} else {
			msg.ReceiveChan <- shardResponse{
				err:              nil,
				reqCtx:           p.cancellationCtx,
				enqueueBroadcast: p.enqueueBroadcast,
			}
			p.log("msg: processed", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
		}
	}
	defer func() {
		p.ended = time.Now()
		p.cancelFunc()
		p.cancellationCtxCancel()
		// mark allocations ready for GC
		p.outOfOrderMsgs = nil
		// make sure the context is cancelled before closing the channels
		<-p.ctx.Done()
		p.emptyChannels(metadata)
		p.log("processor: stopped", nil, logging.Started(p.started), logging.Ended(p.ended))
	}()
	for {
		select {
		case <-p.ctx.Done():
			return
		case bMsg := <-p.broadcastChan:
			if p.broadcastID != bMsg.BroadcastID {
				p.log("broadcast: wrong BroadcastID", BroadcastIDErr{}, logging.MsgType(bMsg.MsgType.String()), logging.Stopping(false))
				continue
			}
			switch bMsg.MsgType {
			case CancellationMsg:
				if p.handleCancellation(bMsg, metadata) {
					return
				}
			case BroadcastMsg:
				if p.handleBroadcast(bMsg, methods, metadata) {
					// methods keeps track of which methods has been broadcasted to.
					// This prevents duplicate broadcasts.
					methods = append(methods, bMsg.Method)
				}
			case ReplyMsg:
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
				p.log("msg: wrong BroadcastID", BroadcastIDErr{}, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
				continue
			}
			if new.IsCancellation {
				// the cancellation implementation is just an
				// empty function and does not need the ctx or
				// broadcastChan.
				new.ReceiveChan <- shardResponse{
					err: nil,
				}
				p.log("msg: received cancellation", nil, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
				continue
			}

			if new.IsBroadcastClient {
				if metadata.HasReceivedClientReq {
					// this is a duplicate request, possibly from a forward operation.
					// the req should simply be dropped.
					new.ReceiveChan <- shardResponse{
						err: ClientReqAlreadyReceivedErr{},
					}
					p.log("msg: duplicate client req", ClientReqAlreadyReceivedErr{}, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
					continue
				}
				// important to set this option to prevent duplicate client reqs.
				// this can be the result if a server forwards the req but the
				// leader has already received the client req.
				metadata.HasReceivedClientReq = true
				go func() {
					// new.Ctx will correspond to the streamCtx between the client and this server.
					// We can thus listen to it and signal a cancellation if the client goes offline
					// or cancels the request. We also have to listen to the p.ctx to prevent leaking
					// the goroutine.
					select {
					case <-p.ctx.Done():
					case <-new.Ctx.Done():
					}
					p.cancellationCtxCancel()
				}()
				p.log("msg: received client req", nil, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
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
				//	slog.Info("receive: late", "err", err, "id", p.broadcastID)
				p.log("msg: late msg", err, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
				return
			}
			if !p.isInOrder(new.CurrentMethod) {
				// save the message and execute it later
				p.addToOutOfOrder(new)
				new.ReceiveChan <- shardResponse{
					err: OutOfOrderErr{},
				}
				p.log("msg: out of order", OutOfOrderErr{}, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
				continue
			}
			new.ReceiveChan <- shardResponse{
				err:              nil,
				reqCtx:           p.cancellationCtx,
				enqueueBroadcast: p.enqueueBroadcast,
			}
			p.log("msg: processed", nil, logging.Method(new.CurrentMethod), logging.From(new.SenderAddr))
		}
	}
}

func (p *BroadcastProcessor) handleCancellation(bMsg *Msg, metadata *metadata) bool {
	if bMsg.Cancellation.end {
		p.log("broadcast: broadcast.Done() called", nil, logging.MsgType(bMsg.MsgType.String()), logging.Stopping(true))
		return true
	}
	if !metadata.SentCancellation {
		p.log("broadcast: sent cancellation", nil, logging.MsgType(bMsg.MsgType.String()), logging.Stopping(false))
		metadata.SentCancellation = true
		go p.router.Send(p.broadcastID, "", "", bMsg.Cancellation)
	}
	return false
}

func (p *BroadcastProcessor) handleBroadcast(bMsg *Msg, methods []string, metadata *metadata) bool {
	// check if msg has already been broadcasted for this method
	//if alreadyBroadcasted(p.metadata.Methods, bMsg.Method) {
	if alreadyBroadcasted(methods, bMsg.Method) {
		return false
	}
	err := p.router.Send(p.broadcastID, metadata.OriginAddr, metadata.OriginMethod, bMsg.Msg)
	p.log("broadcast: sending broadcast", err, logging.MsgType(bMsg.MsgType.String()), logging.Method(bMsg.Method), logging.Stopping(false), logging.IsBroadcastCall(metadata.isBroadcastCall()))

	p.updateOrder(bMsg.Method, bMsg.Msg.options.ProgressTo)
	p.dispatchOutOfOrderMsgs()
	return true
}

func (p *BroadcastProcessor) handleReply(bMsg *Msg, metadata *metadata) bool {
	// BroadcastCall if origin addr is non-empty.
	if metadata.isBroadcastCall() {
		go p.router.Send(p.broadcastID, metadata.OriginAddr, metadata.OriginMethod, bMsg.Reply)
		// the request is done becuase we have sent a reply to the client
		p.log("broadcast: sending reply to client", nil, logging.Method(metadata.OriginMethod), logging.MsgType(bMsg.MsgType.String()), logging.Stopping(true), logging.IsBroadcastCall(metadata.isBroadcastCall()))
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
		//slog.Info("reply: late", "err", err, "id", p.broadcastID)
		p.log("broadcast: failed to send reply to client", err, logging.Method(metadata.OriginMethod), logging.MsgType(bMsg.MsgType.String()), logging.Stopping(false), logging.IsBroadcastCall(metadata.isBroadcastCall()))
		// we must stop the goroutine if we have received the client req. This can mean that
		// the client no longer accepts replies or has gone offline. In any case, the operation
		// is done.
		return metadata.hasReceivedClientRequest()
	}
	// the request is done becuase we have sent a reply to the client
	p.log("broadcast: sending reply to client", err, logging.Method(metadata.OriginMethod), logging.MsgType(bMsg.MsgType.String()), logging.Stopping(true), logging.IsBroadcastCall(metadata.isBroadcastCall()))
	return true
}

func (p *BroadcastProcessor) log(msg string, err error, args ...slog.Attr) {
	if p.logger != nil {
		args = append(args, logging.Err(err), logging.Type("broadcast processor"))
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}
		p.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}

func (m *metadata) update(new *Content) {
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

func (p *BroadcastProcessor) emptyChannels(metadata *metadata) {
	for {
		select {
		case msg := <-p.broadcastChan:
			if p.broadcastID != msg.BroadcastID {
				continue
			}
			switch msg.MsgType {
			case CancellationMsg:
				// it is possible to call SendToClient() before Cancel() in the same
				// server handler. Since SendToClient() will stop the processor, we need
				// to handle the cancellation here. We don't want to send duplicate
				// cancellations and thus we only want to send a cancellation if the
				// request has been stopped.
				p.handleCancellation(msg, metadata)
			case BroadcastMsg:
				// broadcasts are not performed after a reply to the client is sent.
				// this is to prevent duplication and processing of old messages.
			case ReplyMsg:
				// a reply should not be sent after the processor is done. This is
				// because either:
				// 	1. A reply has already been sent
				// 	2. Done() has been called. SendToClient() should not be used together
				//	   with this method
			}
		default:
			return
		}
	}
}

func (r *BroadcastProcessor) initOrder() {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	r.outOfOrderMsgs = make(map[string][]*Content)
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

func (r *BroadcastProcessor) addToOutOfOrder(msg *Content) {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	var (
		msgs []*Content
		ok   bool
	)
	if msgs, ok = r.outOfOrderMsgs[msg.CurrentMethod]; ok {
		msgs = append(msgs, msg)
	} else {
		msgs = []*Content{msg}
	}
	r.outOfOrderMsgs[msg.CurrentMethod] = msgs
}

func (r *BroadcastProcessor) updateOrder(method string, progressTo string) {
	// the implementer has not specified an execution order
	if r.executionOrder == nil || len(r.executionOrder) <= 0 {
		return
	}
	if progressTo != "" {
		method = progressTo
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
				r.log("msg: dispatching out of order msg", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
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
// of a broadcast processor. The messages enqueued are then transmitted
// to the other servers or the client depending on the type of message.
// Currently there are three types:
// - BroadcastMsg
// - ClientReply
// - Cancellation
func (p *BroadcastProcessor) enqueueBroadcast(msg *Msg) error {
	// we want to prevent queueing messages on the buffered broadcastChan
	// because it can potentially lead to concurrency bugs. These include:
	//	- buffering a message on the channel and requiring that it is processed.
	//	  this can happen with cancellation when SendToClient() is called first.
	// 	- reaching the end of the buffer (same as not buffering the channel) and
	//	  closing the broadcastChan at the same time. This will cause an error.
	select {
	case <-p.ctx.Done():
		p.log("broadcast: already processed", AlreadyProcessedErr{}, logging.Method(msg.Method), logging.MsgType(msg.MsgType.String()))
		return AlreadyProcessedErr{}
	default:
	}
	// this is not an optimal solution regarding cancellations. The cancellation
	// msg can be discarded if the buffer is fully populated. This is because
	// ctx.Done() will be called before the msg is queued.
	select {
	case <-p.ctx.Done():
		p.log("broadcast: already processed", AlreadyProcessedErr{}, logging.Method(msg.Method), logging.MsgType(msg.MsgType.String()))
		return AlreadyProcessedErr{}
	case p.broadcastChan <- msg:
		return nil
	}
}

func alreadyBroadcasted(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func (c *Content) isBroadcastCall() bool {
	return c.OriginAddr != ""
}

func (c *Content) hasReceivedClientRequest() bool {
	return c.IsBroadcastClient && c.SendFn != nil
}
