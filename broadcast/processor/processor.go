package processor

import (
	"context"
	"github.com/relab/gorums/broadcast/dtos"
	errs "github.com/relab/gorums/broadcast/errors"
	"github.com/relab/gorums/broadcast/router"
	"log/slog"
	"time"

	"github.com/relab/gorums/logging"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Processor struct {
	router        router.Router
	broadcastChan chan dtos.Msg
	sendChan      chan *RequestDto
	ctx           context.Context
	cancelFunc    context.CancelFunc
	logger        *slog.Logger
	metadata      *metadata
}

type Config struct {
	Ctx             context.Context
	CancelFunc      context.CancelFunc
	SendBuffer      int
	Router          router.Router
	ExecutionOrder  map[string]int
	Logger          *slog.Logger
	BroadcastID     uint64
	OriginAddr      string
	OriginMethod    string
	IsServer        bool
	SendFn          func(protoreflect.ProtoMessage, error) error
	OriginDigest    []byte
	OriginSignature []byte
	OriginPubKey    string
}

type metadata struct {
	broadcastID          uint64
	originAddr           string
	originMethod         string
	sent                 bool
	responseMsg          protoreflect.ProtoMessage
	responseErr          error
	sendFn               func(protoreflect.ProtoMessage, error) error
	isBroadcastClient    bool
	hasReceivedClientReq bool
	originDigest         []byte
	originPubKey         string
	originSignature      []byte
	methods              []string
	started              time.Time
	ended                time.Time

	// ordering
	executionOrder map[string]int
	orderIndex     int
	outOfOrderMsgs map[string][]*RequestDto
}

func New(config *Config) *Processor {
	return &Processor{
		ctx:           config.Ctx,
		cancelFunc:    config.CancelFunc,
		sendChan:      make(chan *RequestDto, config.SendBuffer),
		broadcastChan: make(chan dtos.Msg, config.SendBuffer),
		router:        config.Router,
		logger:        config.Logger,
		metadata: &metadata{
			broadcastID:       config.BroadcastID,
			originAddr:        config.OriginAddr,
			originMethod:      config.OriginMethod,
			isBroadcastClient: config.IsServer,
			sendFn:            config.SendFn,
			sent:              false,
			originDigest:      config.OriginDigest,
			originSignature:   config.OriginSignature,
			originPubKey:      config.OriginPubKey,
			methods:           make([]string, 0, 3),
			started:           time.Now(),
			executionOrder:    config.ExecutionOrder,
		},
	}
}

func (p *Processor) Start(msg *RequestDto) {
	p.initialize(msg)
	defer p.cleanup()
	p.run()
}

func (p *Processor) run() {
	for {
		select {
		case <-p.ctx.Done():
			// processor is done
			return
		case internal := <-p.broadcastChan:
			// we have received an outgoing message from a server handler
			switch m := internal.(type) {
			case *dtos.BroadcastMsg:
				p.handleBroadcast(m)
			case *dtos.ReplyMsg:
				if p.handleReply(m) {
					// request is done if a reply is sent to the client.
					return
				}
			}
		case external := <-p.sendChan:
			// we have received an external message from a server or client
			if p.handleMsg(external) {
				return
			}
		}
	}
}

func (p *Processor) handleBroadcast(msg *dtos.BroadcastMsg) {
	// check if msg has already been broadcast for this method
	// if alreadyBroadcast(p.metadata.methods, msg.Method) {
	if !msg.Options.AllowDuplication && p.alreadyBroadcast(msg) {
		return
	}

	err := p.sendBroadcastMsg(msg)
	p.log("broadcast: sending broadcast", err, logging.MsgType("broadcast"), logging.Method(msg.Info.Method), logging.Stopping(false), logging.IsBroadcastCall(p.metadata.isBroadcastCall()))

	// methods keeps track of which methods has been broadcast to.
	// This prevents duplicate broadcasts.
	p.metadata.methods = append(p.metadata.methods, msg.Info.Method)
	p.updateOrder(msg.Info.Method, msg.Options.ProgressTo)
	p.dispatchOutOfOrderMsgs()
}

func (p *Processor) handleReply(reply *dtos.ReplyMsg) bool {
	// BroadcastCall if origin Addr is non-empty.
	if p.metadata.isBroadcastCall() {
		go func() {
			err := p.sendReplyMsg(reply)
			p.log("broadcast: sent reply to client", err, logging.Method(p.metadata.originMethod), logging.MsgType("reply"), logging.Stopping(true), logging.IsBroadcastCall(p.metadata.isBroadcastCall()))
		}()
		// the request is done because we have sent a reply to the client
		p.log("broadcast: sending reply to client", nil, logging.Method(p.metadata.originMethod), logging.MsgType("reply"), logging.Stopping(true), logging.IsBroadcastCall(p.metadata.isBroadcastCall()))
		return true
	}
	// QuorumCall if origin Addr is empty.

	// this sends a reply back to the client only if the client has
	// connected to the server. Otherwise, an error will be returned.
	// We thus need to cache the msg until the client has connected to
	// the server.
	err := p.metadata.send(reply.Info.Message, reply.Err)
	if err != nil {
		// add response if not already done
		if p.metadata.responseMsg == nil {
			p.metadata.responseMsg = reply.Info.Message
			p.metadata.responseErr = reply.Err
			p.metadata.sent = true
		}
		// the request is not done yet because we have not replied to
		// the client.
		p.log("broadcast: failed to send reply to client", err, logging.Method(p.metadata.originMethod), logging.MsgType("reply"), logging.Stopping(false), logging.IsBroadcastCall(p.metadata.isBroadcastCall()))
		// we must stop the goroutine if we have received the client req. This can mean that
		// the client no longer accepts replies or has gone offline. In any case, the operation
		// is done.
		return p.metadata.hasReceivedClientRequest()
	}
	// the request is done because we have sent a reply to the client
	p.log("broadcast: sending reply to client", err, logging.Method(p.metadata.originMethod), logging.MsgType("reply"), logging.Stopping(true), logging.IsBroadcastCall(p.metadata.isBroadcastCall()))
	return true
}

func (p *Processor) handleMsg(dto *RequestDto) bool {
	if p.metadata.broadcastID != dto.BroadcastID {
		p.log("dto: wrong BroadcastID", errs.BroadcastIDErr{}, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
		return false
	}
	if !dto.IsServer {
		if p.metadata.hasReceivedClientReq {
			// this is a duplicate request, possibly from a forward operation.
			// the req should simply be dropped.
			p.log("dto: duplicate client req", errs.ClientReqAlreadyReceivedErr{}, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
			return false
		}
		// important to set this option to prevent duplicate client reqs.
		// this can be the result if a server forwards the req but the
		// leader has already received the client req.
		p.metadata.hasReceivedClientReq = true
		p.log("dto: received client req", nil, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
	}

	p.metadata.update(dto)
	// this only pertains to requests where the server has a
	// direct connection to the client, e.g. QuorumCall.
	if p.metadata.sent && !p.metadata.isBroadcastCall() {
		// we must return an error to prevent executing the implementation func.
		// This is because the server has finished the request and tried to reply
		// to the client previously. The dto we have just received is from the client,
		// meaning we can finally return the cached response.
		err := p.metadata.send(p.metadata.responseMsg, p.metadata.responseErr)
		p.log("dto: late dto", err, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
		return true
	}
	if !p.isInOrder(dto.CurrentMethod) {
		// save the message and execute it later
		p.addToOutOfOrder(dto)
		p.log("dto: out of order", errs.OutOfOrderErr{}, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
		return false
	}
	dto.Run(p.enqueueMsg)
	p.log("dto: processed", nil, logging.Method(dto.CurrentMethod), logging.From(dto.SenderAddr))
	return false
}

func (p *Processor) log(msg string, err error, args ...slog.Attr) {
	if p.logger != nil {
		args = append(args, logging.Err(err), logging.Type("broadcast processor"))
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}
		p.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}

func (m *metadata) update(new *RequestDto) {
	if m.originAddr == "" && new.OriginAddr != "" {
		m.originAddr = new.OriginAddr
	}
	if m.originMethod == "" && new.OriginMethod != "" {
		m.originMethod = new.OriginMethod
	}
	if m.originPubKey == "" && new.OriginPubKey != "" {
		m.originPubKey = new.OriginPubKey
	}
	if m.originSignature == nil && new.OriginSignature != nil {
		m.originSignature = new.OriginSignature
	}
	if m.originDigest == nil && new.OriginDigest != nil {
		m.originDigest = new.OriginDigest
	}
	if m.sendFn == nil && new.SendFn != nil {
		m.sendFn = new.SendFn
		m.isBroadcastClient = new.IsServer
	}
}

func (p *Processor) sendBroadcastMsg(dto *dtos.BroadcastMsg) error {
	dto.OriginAddr = p.metadata.originAddr
	dto.Info.OriginMethod = p.metadata.originMethod
	dto.Info.OriginDigest = p.metadata.originDigest
	dto.Info.OriginSignature = p.metadata.originSignature
	dto.Info.OriginPubKey = p.metadata.originPubKey
	return p.router.Broadcast(dto)
}

func (p *Processor) sendReplyMsg(dto *dtos.ReplyMsg) error {
	dto.ClientAddr = p.metadata.originAddr
	dto.Info.OriginMethod = p.metadata.originMethod
	dto.Info.OriginDigest = p.metadata.originDigest
	dto.Info.OriginSignature = p.metadata.originSignature
	dto.Info.OriginPubKey = p.metadata.originPubKey
	return p.router.ReplyToClient(dto)
}

func (m *metadata) isBroadcastCall() bool {
	return m.originAddr != ""
}

func (m *metadata) send(resp protoreflect.ProtoMessage, err error) error {
	if !m.hasReceivedClientRequest() {
		return errs.MissingClientReqErr{}
	}
	// error is intentionally ignored. We have not setup retry logic for failed
	// deliveries to clients. Responding with nil will stop the broadcast request
	// which is needed to prevent many stale goroutines.
	_ = m.sendFn(resp, err)
	return nil
}

func (m *metadata) hasReceivedClientRequest() bool {
	return m.isBroadcastClient && m.sendFn != nil
}

func (p *Processor) emptyChannels() {
	for {
		select {
		case <-p.broadcastChan:
		default:
			return
		}
	}
}

func (p *Processor) initOrder() {
	// the implementer has not specified an execution order
	if p.metadata.executionOrder == nil || len(p.metadata.executionOrder) <= 0 {
		return
	}
	p.metadata.outOfOrderMsgs = make(map[string][]*RequestDto)
}

func (p *Processor) isInOrder(method string) bool {
	// the implementer has not specified an execution order
	if p.metadata.executionOrder == nil || len(p.metadata.executionOrder) <= 0 {
		return true
	}
	order, ok := p.metadata.executionOrder[method]
	// accept all methods without a specified order
	if !ok {
		return true
	}
	// the first method should always be allowed to be executed
	if p.metadata.executionOrder[method] <= 0 {
		return true
	}
	return order <= p.metadata.orderIndex
}

func (p *Processor) addToOutOfOrder(msg *RequestDto) {
	// the implementer has not specified an execution order
	if p.metadata.executionOrder == nil || len(p.metadata.executionOrder) <= 0 {
		return
	}
	var (
		msgs []*RequestDto
		ok   bool
	)
	if msgs, ok = p.metadata.outOfOrderMsgs[msg.CurrentMethod]; ok {
		msgs = append(msgs, msg)
	} else {
		msgs = []*RequestDto{msg}
	}
	p.metadata.outOfOrderMsgs[msg.CurrentMethod] = msgs
}

func (p *Processor) updateOrder(method string, progressTo string) {
	// the implementer has not specified an execution order
	if p.metadata.executionOrder == nil || len(p.metadata.executionOrder) <= 0 {
		return
	}
	if progressTo != "" {
		method = progressTo
	}
	order, ok := p.metadata.executionOrder[method]
	// do nothing for methods without specified order
	if !ok {
		return
	}
	if order > p.metadata.orderIndex {
		p.metadata.orderIndex = order
	}
}

func (p *Processor) dispatchOutOfOrderMsgs() {
	// the implementer has not specified an execution order
	if p.metadata.executionOrder == nil || len(p.metadata.executionOrder) <= 0 {
		return
	}
	// return early if there are no cached msgs
	if len(p.metadata.outOfOrderMsgs) <= 0 {
		return
	}
	handledMethods := make([]string, 0, len(p.metadata.outOfOrderMsgs))
	for method, msgs := range p.metadata.outOfOrderMsgs {
		order, ok := p.metadata.executionOrder[method]
		if !ok {
			// this should not be possible unless the execution order
			// is changed during operation, which is prohibited.
			panic("how did you get here?")
		}
		if order <= p.metadata.orderIndex {
			for _, msg := range msgs {
				msg.Run(p.enqueueMsg)
				p.log("msg: dispatching out of order msg", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
			}
			handledMethods = append(handledMethods, method)
		}
	}
	// cleanup after dispatching the cached messages
	for _, m := range handledMethods {
		delete(p.metadata.outOfOrderMsgs, m)
	}
}

func (p *Processor) alreadyBroadcast(msg *dtos.BroadcastMsg) bool {
	for _, m := range p.metadata.methods {
		if m == msg.Info.Method {
			return true
		}
	}
	return false
}

func (p *Processor) initialize(msg *RequestDto) {
	p.log("processor: started", nil, logging.Started(p.metadata.started))
	p.initOrder()
	// connect to client immediately to potentially save some time
	go p.router.Connect(p.metadata.originAddr)
	if !msg.IsServer {
		// important to set this option to prevent duplicate client reqs.
		// this can be the result if a server forwards the req but the
		// leader has already received the client req.
		p.metadata.hasReceivedClientReq = true
		p.log("msg: received client req", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
	}
	if !p.isInOrder(msg.CurrentMethod) {
		// save the message and execute it later
		p.addToOutOfOrder(msg)
		p.log("msg: out of order", errs.OutOfOrderErr{}, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
	} else {
		msg.Run(p.enqueueMsg)
		p.log("msg: processed", nil, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
	}
}

// cleanup reduces the memory footprint of the broadcast processor. We still want to keep a reference to the broadcast
// processor for a while (in one of the shards) because messages pertaining to this processor may arrive after it has
// been finished. Hence, to prevent creating a new broadcast processor we instead keep a reference to this (finished)
// processor for a limited amount of time.
func (p *Processor) cleanup() {
	p.metadata.ended = time.Now()
	p.cancelFunc()
	// make sure the context is cancelled before closing the channels
	<-p.ctx.Done()
	p.emptyChannels()
	// mark allocations ready for GC and minimize memory footprint of finished broadcast requests.
	// this is safe to do because all send operations listen to the cancelled p.ctx thus preventing
	// deadlocks/goroutine leaks.
	p.metadata = nil
	/*
		These two lines would greatly impact the amount of memory used by a processor:
		1. p.broadcastChan = nil
		2. p.sendChan = nil

		However, the shard uses them, and thus we get a race condition when setting them to nil.
		Using mutexes will remove the benefit of setting them to nil. It is safe to set them to
		nil because sending on a nil channel will block forever. We always listen to ctx.Done(),
		ensuring that we don't get deadlocks. Additionally, queued msgs will be dropped in any
		case and new msgs will not even get enqueued. As a concluding remark, we comment the lines
		out to not get hits when running tests with the race detector.
	*/
	p.log("processor: stopped", nil, logging.Started(p.metadata.started), logging.Ended(p.metadata.ended))
}

// EnqueueMsg is the function that enqueues a message (either broadcast or client reply) to be processed by a broadcast
// processor. Also, this function is ONLY called from a user implemented server method.
//
// NOTE: The only implementation of this type should be located immediately below this definition.
type EnqueueMsg func(msg dtos.Msg) error

// this method is used to enqueue messages onto the broadcast channel
// of a broadcast processor. The messages enqueued are then transmitted
// to the other servers or the client depending on the type of message.
// Currently, there are three types:
// - BroadcastMsg
// - ClientReply
// - Cancellation
func (p *Processor) enqueueMsg(msg dtos.Msg) error {
	if p.metadata.broadcastID != msg.GetBroadcastID() {
		p.log("broadcast: wrong BroadcastID", errs.BroadcastIDErr{}, logging.MsgType(msg.String()), logging.Stopping(false))
		return errs.BroadcastIDErr{}
	}
	// we want to prevent queueing messages on the buffered broadcastChan
	// because it can potentially lead to concurrency bugs. These include:
	//	- buffering a message on the channel and requiring that it is processed.
	//	  this can happen with cancellation when SendToClient() is called first.
	// 	- reaching the end of the buffer (same as not buffering the channel) and
	//	  closing the broadcastChan at the same time. This will cause an error.
	select {
	case <-p.ctx.Done():
		p.log("broadcast: already processed", errs.AlreadyProcessedErr{}, logging.Method(msg.GetMethod()), logging.MsgType(msg.String()))
		return errs.AlreadyProcessedErr{}
	default:
	}
	// this is not an optimal solution regarding cancellations. The cancellation
	// msg can be discarded if the buffer is fully populated. This is because
	// ctx.Done() will be called before the msg is queued.
	select {
	case <-p.ctx.Done():
		p.log("broadcast: already processed", errs.AlreadyProcessedErr{}, logging.Method(msg.GetMethod()), logging.MsgType(msg.String()))
		return errs.AlreadyProcessedErr{}
	case p.broadcastChan <- msg:
		return nil
	}
}

func (p *Processor) GetEnqueueMsgFunc() EnqueueMsg {
	return p.enqueueMsg
}

func (p *Processor) IsFinished(msg *RequestDto) bool {
	select {
	case <-p.ctx.Done():
		p.log("msg: already processed", errs.AlreadyProcessedErr{}, logging.Method(msg.CurrentMethod), logging.From(msg.SenderAddr))
		return true
	default:
	}
	return false
}

func (p *Processor) EnqueueExternalMsg(msg *RequestDto) {
	// must check if the req is done to prevent deadlock
	select {
	case <-p.ctx.Done():
	case p.sendChan <- msg:
	}
}
