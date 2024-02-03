package gorums

import (
	"context"
	"errors"
	reflect "reflect"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastFunc2 func(ctx context.Context, req RequestTypes) (resp ResponseTypes, err error)
type BroadcastFunc func(ctx context.Context, req RequestTypes, broadcastID string)
type ConversionFunc func(ctx context.Context, req any) any

type defaultImplementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)
type implementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error, bool)
type implementationFunc2[T RequestTypes, V ResponseTypes, U RequestTypes] func(ServerCtx, T, func(U)) (V, error)
type implementationFunc3[T RequestTypes, V ResponseTypes] func(ServerCtx, T, func(V)) (V, error)
type implementationFunc4[T RequestTypes, V ResponseTypes, U broadcastStruct] func(ServerCtx, T, U) (V, error)
type implementationFunc5[T RequestTypes, V broadcastStruct] func(ServerCtx, T, V) error

func DefaultHandler[T RequestTypes, V ResponseTypes](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

type broadcastMsg interface {
	GetFrom() string
	GetRequest() RequestTypes
	GetMethod() string
	GetContext() ServerCtx
	GetBroadcastID() string
}

type broadcastMessage[T RequestTypes, V ResponseTypes] struct {
	from           string
	request        T
	implementation implementationFunc[T, V]
	method         string
	context        ServerCtx
	broadcastID    string
}

func (b *broadcastMessage[T, V]) GetFrom() string {
	return b.from
}

func (b *broadcastMessage[T, V]) GetRequest() RequestTypes {
	return b.request
}

func (b *broadcastMessage[T, V]) GetImplementation() implementationFunc[T, V] {
	return b.implementation
}

func (b *broadcastMessage[T, V]) GetMethod() string {
	return b.method
}

func (b *broadcastMessage[T, V]) GetContext() ServerCtx {
	return b.context
}

func (b *broadcastMessage[T, V]) GetBroadcastID() string {
	return b.broadcastID
}

func newBroadcastMessage[T RequestTypes, V ResponseTypes](ctx ServerCtx, req T, method string, broadcastID string) *broadcastMessage[T, V] {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage[T, V]{
		from:        addr,
		request:     req,
		method:      method,
		context:     ctx,
		broadcastID: broadcastID,
	}
}

type broadcastMessage2 struct {
	from        string
	request     RequestTypes
	method      string
	context     ServerCtx
	broadcastID string
}

func (b *broadcastMessage2) GetFrom() string {
	return b.from
}

func (b *broadcastMessage2) GetRequest() RequestTypes {
	return b.request
}

func (b *broadcastMessage2) GetMethod() string {
	return b.method
}

func (b *broadcastMessage2) GetContext() ServerCtx {
	return b.context
}

func (b *broadcastMessage2) GetBroadcastID() string {
	return b.broadcastID
}

func newBroadcastMessage2(ctx ServerCtx, req RequestTypes, method string, broadcastID string) *broadcastMessage2 {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage2{
		from:        addr,
		request:     req,
		method:      method,
		context:     ctx,
		broadcastID: broadcastID,
	}
}

func BroadcastHandler3[T RequestTypes, V ResponseTypes](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		/*if srv.shouldBroadcast(in.Metadata.MessageID) {
			// need to know:
			//	- which method to call on the other servers
			//	- who sent the message
			//	- the type of the message
			//broadcastChan <- newBroadcastMessage[T, V](ctx, req, impl)
			//go srv.broadcast(newBroadcastMessage[T, V](ctx, req, impl, in.Metadata.Method))
		}*/
		/*var resp responseTypes
		var err error
		var broadcast bool*/
		//fmt.Println("sender:", in.Metadata.Sender, in.Metadata.Method, "round:", in.Metadata.BroadcastID)
		resp, err, broadcast := impl(ctx, req)
		if broadcast && !srv.alreadyBroadcasted(in.Metadata.BroadcastMsg.BroadcastID, in.Metadata.Method) {
			go srv.broadcast(newBroadcastMessage[T, V](ctx, req, in.Metadata.Method, in.Metadata.BroadcastMsg.BroadcastID))
		}
		/*if !srv.alreadyReceivedFromPeer(ctx, in.Metadata.MessageID, in.Metadata.BroadcastID, in.Metadata.Method, in.Metadata.Sender) {
			resp, err, broadcast = impl(ctx, req)
			if broadcast && !srv.alreadyBroadcasted(in.Metadata.MessageID) {
				srv.broadcastedMsgs[in.Metadata.MessageID] = true
				go srv.broadcast(newBroadcastMessage[T, V](ctx, req, impl, in.Metadata.Method, in.Metadata.BroadcastID))
			}
		}*/
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

type responseMsg interface {
	GetResponse() ResponseTypes
	GetError() error
	GetBroadcastID() string
	Valid() bool
	GetType() respType
}

type respType int

const (
	unhandled respType = iota
	clientResponse
	timeout
	done
)

type responseMessage struct {
	response    ResponseTypes
	err         error
	broadcastID string
	timestamp   time.Time
	ttl         time.Duration
	respType    respType
}

func newResponseMessage(response ResponseTypes, err error, broadcastID string, respType respType, ttl time.Duration) *responseMessage {
	return &responseMessage{
		response:    response,
		err:         err,
		broadcastID: broadcastID,
		timestamp:   time.Now(),
		ttl:         ttl,
		respType:    respType,
	}
}

func (r *responseMessage) GetResponse() ResponseTypes {
	return r.response
}

func (r *responseMessage) GetError() error {
	return r.err
}

func (r *responseMessage) GetBroadcastID() string {
	return r.broadcastID
}

func (r *responseMessage) Valid() bool {
	return r.respType == clientResponse && time.Since(r.timestamp) <= r.ttl
}

func (r *responseMessage) GetType() respType {
	return r.respType
}

func ReturnToClientHandler[T RequestTypes, V ResponseTypes](impl implementationFunc3[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		var returnToClient *bool = new(bool)
		*returnToClient = false
		response := new(V)
		resp, err := impl(ctx, req, determineReturnToClient[V](returnToClient, response))
		if *returnToClient && !srv.alreadyReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, in.Metadata.Method) {
			srv.setReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, true)
			go func() {
				// err must be sent to user similar to response?
				srv.responseChan <- newResponseMessage(*response, err, in.Metadata.BroadcastMsg.BroadcastID, clientResponse, srv.timeout)
			}()
		}
		// server to server communication does not need response?
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func determineReturnToClient[V ResponseTypes](shouldReturnToClient *bool, response *V) func(V) {
	return func(resp V) {
		*shouldReturnToClient = true
		*response = resp
	}
}

func BroadcastHandler4[T RequestTypes, V ResponseTypes, U broadcastStruct](impl implementationFunc4[T, V, U], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		//var broadcast *bool = new(bool)
		//*broadcast = false
		//request := new(U)
		//resp, err := impl(ctx, req, determineBroadcast2[U](broadcast, request, srv))
		srv.b.Reset()
		resp, err := impl(ctx, req, srv.b.(U))
		//if *broadcast && !srv.alreadyBroadcasted(in.Metadata.BroadcastID, in.Metadata.Method) {
		if srv.b.ShouldBroadcast() && !srv.alreadyBroadcasted(in.Metadata.BroadcastMsg.BroadcastID, srv.b.GetMethod()) {
			// how to define individual request message to each node?
			//	- maybe create one request for each node and send a list of requests?
			go srv.broadcast(newBroadcastMessage2(ctx, srv.b.GetRequest(), srv.b.GetMethod(), in.Metadata.BroadcastMsg.BroadcastID))
		}
		if srv.b.ShouldReturnToClient() && !srv.alreadyReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, srv.b.GetMethod()) {
			srv.setReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, true)
			go func() {
				// err must be sent to user similar to response?
				srv.responseChan <- newResponseMessage(srv.b.GetResponse(), srv.b.GetError(), in.Metadata.BroadcastMsg.BroadcastID, clientResponse, srv.timeout)
			}()
		}
		// verify whether a server or a client sent the request
		if in.Metadata.BroadcastMsg.Sender == "client" {
			go determineClientResponse[V](srv, ctx, in, finished, resp, err)
		} else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
		}
	}
}

func (srv *Server) RegisterMiddlewares(middlewares ...func()) {
	srv.middlewares = middlewares
}

func (srv *Server) runMiddleware() {
	for _, middleware := range srv.middlewares {
		middleware()
	}
}

func BroadcastHandler[T RequestTypes, V broadcastStruct](impl implementationFunc5[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		//log.Println("BroadcastID:", in.Metadata.BroadcastMsg.BroadcastID, "Method:", in.Metadata.Method)
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		//var broadcast *bool = new(bool)
		//*broadcast = false
		//request := new(U)
		//resp, err := impl(ctx, req, determineBroadcast2[U](broadcast, request, srv))
		srv.runMiddleware()
		srv.b.Reset(in.Metadata.BroadcastMsg.BroadcastID)
		_ = impl(ctx, req, srv.b.(V))
		//if *broadcast && !srv.alreadyBroadcasted(in.Metadata.BroadcastID, in.Metadata.Method) {
		if srv.b.ShouldBroadcast() && !srv.alreadyBroadcasted(in.Metadata.BroadcastMsg.BroadcastID, srv.b.GetMethod()) {
			// how to define individual request message to each node?
			//	- maybe create one request for each node and send a list of requests?
			go srv.broadcast(newBroadcastMessage2(ctx, srv.b.GetRequest(), srv.b.GetMethod(), in.Metadata.BroadcastMsg.BroadcastID))
		}
		if srv.b.ShouldReturnToClient() && !srv.alreadyReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, srv.b.GetMethod()) {
			srv.setReturnedToClient(in.Metadata.BroadcastMsg.BroadcastID, true)
			go func() {
				srv.responseChan <- newResponseMessage(srv.b.GetResponse(), srv.b.GetError(), in.Metadata.BroadcastMsg.BroadcastID, clientResponse, srv.timeout)
			}()
		}
		// verify whether a server or a client sent the request
		if in.Metadata.BroadcastMsg.Sender == "client" {
			srv.addClientRequest(in.Metadata, ctx, finished)
			go srv.timeoutClientResponse(ctx, in, finished)
			//go determineClientResponse2(srv, ctx, in, finished)
		} /*else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(nil), err))
		}*/
	}
}

type clientRequest struct {
	id       string
	ctx      ServerCtx
	finished chan<- *Message
	metadata *ordering.Metadata
	status   respType
}

type broadcastStruct interface {
	GetMethod() string
	ShouldBroadcast() bool
	ShouldReturnToClient() bool
	Reset(...string)
	GetRequest() RequestTypes
	GetResponse() ResponseTypes
	GetError() error
	GetBroadcastID() string
}

type BroadcastStruct struct {
	Method                  string // could make this a slice to support multiple broadcasts in one gRPC method
	ShouldBroadcastVal      bool
	ShouldReturnToClientVal bool
	Req                     RequestTypes // could make this a slice to support multiple broadcasts in one gRPC method
	Resp                    ResponseTypes
	Err                     error // part of client response
	BroadcastID             string
}

func NewBroadcastStruct() *BroadcastStruct {
	return &BroadcastStruct{}
}

func (b *BroadcastStruct) SetBroadcastValues(method string, req RequestTypes) {
	b.Method = method
	b.ShouldBroadcastVal = true
	b.Req = req
}

func (b *BroadcastStruct) SetReturnToClient(resp ResponseTypes, err error) {
	b.Method = "client"
	b.ShouldReturnToClientVal = true
	b.Resp = resp
	b.Err = err
}

func (b *BroadcastStruct) GetMethod() string {
	return b.Method
}
func (b *BroadcastStruct) GetRequest() RequestTypes {
	return b.Req
}
func (b *BroadcastStruct) GetResponse() ResponseTypes {
	return b.Resp
}
func (b *BroadcastStruct) ShouldBroadcast() bool {
	return b.ShouldBroadcastVal
}
func (b *BroadcastStruct) ShouldReturnToClient() bool {
	return b.ShouldReturnToClientVal
}
func (b *BroadcastStruct) GetError() error {
	return b.Err
}
func (b *BroadcastStruct) GetBroadcastID() string {
	return b.BroadcastID
}
func (b *BroadcastStruct) Reset(broadcastID ...string) {
	b.Method = ""
	b.ShouldBroadcastVal = false
	b.ShouldReturnToClientVal = false
	b.Req = nil
	b.Resp = nil
	b.Err = nil
	if len(broadcastID) >= 1 {
		b.BroadcastID = broadcastID[0]
	} else {
		b.BroadcastID = ""
	}
}

func (srv *Server) RegisterBroadcastStruct(b broadcastStruct) {
	srv.b = b
}

func BroadcastHandler2[T RequestTypes, V ResponseTypes, U RequestTypes](impl implementationFunc2[T, V, U], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		// this will block all broadcast gRPC functions. E.g. if Write and Read are both broadcast gRPC functions. Only one Read or Write can be executed at a time.
		// Maybe implement a per function lock?
		srv.Lock()
		defer srv.Unlock()
		req := in.Message.(T)
		defer ctx.Release()
		var broadcast *bool = new(bool)
		*broadcast = false
		request := new(U)
		resp, err := impl(ctx, req, determineBroadcast[U](broadcast, request))
		if *broadcast && !srv.alreadyBroadcasted(in.Metadata.BroadcastMsg.BroadcastID, in.Metadata.Method) {
			// how to define individual request message to each node?
			//	- maybe create one request for each node and send a list of requests?
			go srv.broadcast(newBroadcastMessage[U, V](ctx, *request, in.Metadata.Method, in.Metadata.BroadcastMsg.BroadcastID))
		}
		// verify whether a server or a client sent the request
		if in.Metadata.BroadcastMsg.Sender == "client" {
			go determineClientResponse[V](srv, ctx, in, finished, resp, err)
		} else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
		}
	}
}

func determineBroadcast[T RequestTypes](shouldBroadcast *bool, request *T) func(T) {
	return func(req T) {
		*shouldBroadcast = true
		*request = req
	}
}

func (srv *Server) RetToClient(resp ResponseTypes, err error, broadcastID string) {
	srv.Lock()
	defer srv.Unlock()
	if !srv.alreadyReturnedToClient(broadcastID, "") {
		srv.setReturnedToClient(broadcastID, true)
		srv.responseChan <- newResponseMessage(resp, err, broadcastID, clientResponse, srv.timeout)
	}
}

func determineClientResponse2(srv *Server, ctx ServerCtx, in *Message, finished chan<- *Message) {
	srv.setReturnedToClient(in.Metadata.BroadcastMsg.GetBroadcastID(), false)
	select {
	case response := <-srv.getResponseToReturnToClient():
		// success
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(response.GetResponse()), response.GetError()))
	case <-time.After(5 * time.Second):
		// fail
		srv.setReturnedToClient(in.Metadata.BroadcastMsg.GetBroadcastID(), true)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(nil), errors.New("server timed out")))
	}
}

func (srv *Server) handleClientResponses() {
	for response := range srv.responseChan {
		req, ok := srv.clientReqs[response.GetBroadcastID()]
		if !ok {
			// this server has not received a request directly from a client
			// hence, the response should be ignored
			continue
		} else if req.status == unhandled {
			// first time it is handled
			req.status = response.GetType()
		} else if req.status != done {
			// already handled, but got the other response type in the pair: clientResponse & timeout
			req.status = done
		} else {
			// already handled and can be removed
			continue
		}
		if !response.Valid() {
			// the response is old and should have timed out, but may not due to scheduling.
			// the timeout msg should arrive soon.
			continue
		}
		SendMessage(req.ctx, req.finished, WrapMessage(req.metadata, protoreflect.ProtoMessage(response.GetResponse()), response.GetError()))
	}
}

func (srv *Server) timeoutClientResponse(ctx ServerCtx, in *Message, finished chan<- *Message) {
	time.After(srv.timeout)
	srv.responseChan <- newResponseMessage(protoreflect.ProtoMessage(nil), errors.New("server timed out"), in.Metadata.BroadcastMsg.GetBroadcastID(), timeout, srv.timeout)
}

func determineClientResponse[V RequestTypes](srv *Server, ctx ServerCtx, in *Message, finished chan<- *Message, resp V, err error) {
	srv.setReturnedToClient(in.Metadata.BroadcastMsg.GetBroadcastID(), false)
	select {
	case response := <-srv.getResponseToReturnToClient():
		// success
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(response.GetResponse()), err))
	case <-time.After(5 * time.Second):
		// fail
		srv.setReturnedToClient(in.Metadata.BroadcastMsg.GetBroadcastID(), true)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func (srv *Server) getResponseToReturnToClient() <-chan responseMsg {
	return srv.responseChan
}

func (srv *Server) alreadyReturnedToClient(broadcastID string, method string) bool {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	returned, ok := srv.returnedToClientMsgs[broadcastID]
	if !ok {
		return true
	}
	return ok && returned
}

func (srv *Server) setReturnedToClient(broadcastID string, val bool) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.returnedToClientMsgs[broadcastID] = val
}

func (srv *Server) addClientRequest(metadata *ordering.Metadata, ctx ServerCtx, finished chan<- *Message) {
	srv.setPendingClientResponse(metadata.BroadcastMsg.GetBroadcastID())
	srv.setReturnedToClient(metadata.BroadcastMsg.GetBroadcastID(), false)
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.clientReqs[metadata.BroadcastMsg.GetBroadcastID()] = &clientRequest{
		id:       uuid.New().String(),
		ctx:      ctx,
		finished: finished,
		metadata: metadata,
		status:   unhandled,
	}
}

func (srv *Server) setPendingClientResponse(broadcastID string) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.pendingClientResponses[broadcastID] = unhandled
}

func (srv *Server) alreadyBroadcasted(broadcastID string, method string) bool {
	_, ok := srv.broadcastedMsgs[broadcastID]
	if !ok {
		srv.broadcastedMsgs[broadcastID] = make(map[string]bool)
	}
	broadcasted, ok := srv.broadcastedMsgs[broadcastID][method]
	if !ok {
		srv.broadcastedMsgs[broadcastID][method] = true
	}
	return ok && broadcasted
}

func (srv *Server) alreadyReceivedFromPeer(ctx ServerCtx, broadcastID, round uint64, method, sender string) bool {
	_, ok := srv.recievedFrom[broadcastID]
	if !ok {
		srv.recievedFrom[broadcastID] = make(map[string]map[string]bool)
	}
	_, ok = srv.recievedFrom[broadcastID][method]
	if !ok {
		srv.recievedFrom[broadcastID][method] = make(map[string]bool)
	}
	// include an addr field in the metadata instead because the ctx address is unreliable in docker networks
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	receivedMsgFromNode, ok := srv.recievedFrom[broadcastID][method][addr]
	if !ok && !receivedMsgFromNode {
		srv.recievedFrom[broadcastID][method][addr] = true
		return false
	}
	return true

}

/*func (srv *Server) shouldBroadcast(msgId uint64) bool {
	_, ok := srv.recievedFrom[msgId]
	if !ok {
		srv.recievedFrom[msgId] = make(map[string]bool)
		return true
	}
	return false
}*/

func (srv *Server) broadcast(broadcastMessage broadcastMsg) {
	//time.Sleep(5 * time.Second)
	// drop if ctx is cancelled? Or in run method?
	srv.BroadcastChan <- broadcastMessage
}

func (srv *Server) ListenForBroadcast() {
	go srv.run()
	go srv.handleClientResponses()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		//*srv.BroadcastID = msg.GetBroadcastID()
		//srv.c.StoreID(msgID-1)
		req := msg.GetRequest()
		method := msg.GetMethod()
		broadcastID := msg.GetBroadcastID()
		ctx := context.Background()
		// reqCtx := msg.GetContext()
		// drop if ctx is cancelled? Or in broadcast method?
		// if another function is called in broadcast, the request needs to be converted
		//if convertFunc, ok := srv.conversions[method]; ok {
		//	convertedReq := convertFunc(ctx, req)
		//	srv.methods[method](ctx, convertedReq)
		//	continue
		//}
		srv.methods[method](ctx, req, broadcastID)
	}
}

func RegisterBroadcastFunc[T RequestTypes, V ResponseTypes](impl func(context.Context, T) (V, error)) func(context.Context, RequestTypes) (ResponseTypes, error) {
	return func(ctx context.Context, req RequestTypes) (resp ResponseTypes, err error) {
		return impl(ctx, req.(T))
	}
}

func RegisterConversionFunc[T RequestTypes, V ResponseTypes](impl func(context.Context, T) V) func(context.Context, any) any {
	return func(ctx context.Context, req any) any {
		return impl(ctx, req.(T))
	}
}

//func (srv *Server) RegisterConversion(method string, conversion func(context.Context, any) any) {
//	srv.conversions[method] = conversion
//}

func (srv *Server) RegisterBroadcastFunc2(method string, broadcastFunc BroadcastFunc2) {
	srv.methods2[method] = broadcastFunc
}

func (srv *Server) RegisterBroadcastFunc(method string) {
	srv.methods[method] = RegisterServerCommunication(srv, method)
}

func (srv *Server) RegisterConfig(c RawConfiguration) {
	srv.config = c
}

func RegisterServerCommunication(srv *Server, method string) func(ctx context.Context, in RequestTypes, broadcastID string) {
	return func(ctx context.Context, in RequestTypes, broadcastID string) {
		cd := BroadcastCallData{
			Message:     in,
			Method:      method,
			BroadcastID: broadcastID,
		}
		srv.config.BroadcastCall(ctx, cd)
		/*cd := QuorumCallData{
			Message:     in,
			Method:      method,
			BroadcastID: broadcastID,
		}
		cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
			return nil, len(replies) >= 3
		}
		srv.config.QuorumCall(ctx, cd)*/
	}
}

func SetDefaultValues[T any](m *T, prefix string) {
	t := reflect.TypeOf(m)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Name
		tag := f.Tag.Get("method")
		s := reflect.ValueOf(m).Elem()
		if s.Kind() != reflect.Struct {
			panic("mapping must be a struct")
		}
		field := s.FieldByName(name)
		if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
			field.SetString(prefix + tag)
		}
	}
}

func (c RawConfiguration) StoreID(val uint64) {
	c[0].mgr.storeID(val)
}

func (m *RawManager) storeID(val uint64) {
	atomic.StoreUint64(&m.nextMsgID, val)
}
