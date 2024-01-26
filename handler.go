package gorums

import (
	"context"
	reflect "reflect"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type requestTypes interface {
	ProtoReflect() protoreflect.Message
}

type responseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastFunc func(ctx context.Context, req requestTypes) (resp responseTypes, err error)
type ConversionFunc func(ctx context.Context, req any) any

type defaultImplementationFunc[T requestTypes, V responseTypes] func(ServerCtx, T) (V, error)
type implementationFunc[T requestTypes, V responseTypes] func(ServerCtx, T) (V, error, bool)
type implementationFunc2[T requestTypes, V responseTypes, U requestTypes] func(ServerCtx, T, func(U)) (V, error)
type implementationFunc3[T requestTypes, V responseTypes] func(ServerCtx, T, func(V)) (V, error)

func DefaultHandler[T requestTypes, V responseTypes](impl defaultImplementationFunc[T, V]) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
	return func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(T)
		defer ctx.Release()
		resp, err := impl(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

type broadcastMsg interface {
	GetFrom() string
	GetRequest() requestTypes
	GetMethod() string
	GetContext() ServerCtx
	GetRound() uint64
}

type broadcastMessage[T requestTypes, V responseTypes] struct {
	from           string
	request        T
	implementation implementationFunc[T, V]
	method         string
	context        ServerCtx
	round          uint64
}

func (b *broadcastMessage[T, V]) GetFrom() string {
	return b.from
}

func (b *broadcastMessage[T, V]) GetRequest() requestTypes {
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

func (b *broadcastMessage[T, V]) GetRound() uint64 {
	return b.round
}

func newBroadcastMessage[T requestTypes, V responseTypes](ctx ServerCtx, req T, method string, round uint64) *broadcastMessage[T, V] {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage[T, V]{
		from:    addr,
		request: req,
		method:  method,
		context: ctx,
		round:   round,
	}
}

func BroadcastHandler[T requestTypes, V responseTypes](impl implementationFunc[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
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
		//fmt.Println("sender:", in.Metadata.Sender, in.Metadata.Method, "round:", in.Metadata.Round)
		resp, err, broadcast := impl(ctx, req)
		if broadcast && !srv.alreadyBroadcasted(in.Metadata.Round, in.Metadata.Method) {
			go srv.broadcast(newBroadcastMessage[T, V](ctx, req, in.Metadata.Method, in.Metadata.Round))
		}
		/*if !srv.alreadyReceivedFromPeer(ctx, in.Metadata.MessageID, in.Metadata.Round, in.Metadata.Method, in.Metadata.Sender) {
			resp, err, broadcast = impl(ctx, req)
			if broadcast && !srv.alreadyBroadcasted(in.Metadata.MessageID) {
				srv.broadcastedMsgs[in.Metadata.MessageID] = true
				go srv.broadcast(newBroadcastMessage[T, V](ctx, req, impl, in.Metadata.Method, in.Metadata.Round))
			}
		}*/
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

type responseMsg interface {
	GetResponse() responseTypes
	GetError() error
}

type responseMessage[V responseTypes] struct {
	response V
	err      error
}

func newResponseMessage[V responseTypes](response V, err error) *responseMessage[V] {
	return &responseMessage[V]{
		response: response,
		err:      err,
	}
}

func (r *responseMessage[V]) GetResponse() responseTypes {
	return r.response
}

func (r *responseMessage[V]) GetError() error {
	return r.err
}

func ReturnToClientHandler[T requestTypes, V responseTypes](impl implementationFunc3[T, V], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
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
		if *returnToClient && !srv.alreadyReturnedToClient(in.Metadata.Round, in.Metadata.Method) {
			srv.setReturnedToClient(in.Metadata.Round, true)
			go func() {
				// err must be sent to user similar to response?
				srv.responseChan <- newResponseMessage[V](*response, err)
			}()
		}
		// server to server communication does not need response?
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func determineReturnToClient[V responseTypes](shouldReturnToClient *bool, response *V) func(V) {
	return func(resp V) {
		*shouldReturnToClient = true
		*response = resp
	}
}

func BroadcastHandler2[T requestTypes, V responseTypes, U requestTypes](impl implementationFunc2[T, V, U], srv *Server) func(ctx ServerCtx, in *Message, finished chan<- *Message) {
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
		if *broadcast && !srv.alreadyBroadcasted(in.Metadata.Round, in.Metadata.Method) {
			// how to define individual request message to each node?
			//	- maybe create one request for each node and send a list of requests?
			go srv.broadcast(newBroadcastMessage[U, V](ctx, *request, in.Metadata.Method, in.Metadata.Round))
		}
		// verify whether a server or a client sent the request
		if in.Metadata.Sender == "client" {
			go determineClientResponse[V](srv, ctx, in, finished, resp, err)
		} else {
			// server to server communication does not need response?
			SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
		}
	}
}

func determineClientResponse[V requestTypes](srv *Server, ctx ServerCtx, in *Message, finished chan<- *Message, resp V, err error) {
	srv.setReturnedToClient(in.Metadata.GetRound(), false)
	select {
	case response := <-srv.getResponseToReturnToClient():
		// success
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(response.GetResponse()), err))
	case <-time.After(5 * time.Second):
		// fail
		srv.setReturnedToClient(in.Metadata.GetRound(), true)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, protoreflect.ProtoMessage(resp), err))
	}
}

func (srv *Server) getResponseToReturnToClient() <-chan responseMsg {
	return srv.responseChan
}

func determineBroadcast[T requestTypes](shouldBroadcast *bool, request *T) func(T) {
	return func(req T) {
		*shouldBroadcast = true
		*request = req
	}
}

func (srv *Server) alreadyReturnedToClient(msgId uint64, method string) bool {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	returned, ok := srv.returnedToClientMsgs[msgId]
	if !ok {
		return true
	}
	return ok && returned
}

func (srv *Server) setReturnedToClient(msgID uint64, val bool) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	srv.returnedToClientMsgs[msgID] = val
}

func (srv *Server) alreadyBroadcasted(msgId uint64, method string) bool {
	_, ok := srv.broadcastedMsgs[msgId]
	if !ok {
		srv.broadcastedMsgs[msgId] = make(map[string]bool)
	}
	broadcasted, ok := srv.broadcastedMsgs[msgId][method]
	if !ok {
		srv.broadcastedMsgs[msgId][method] = true
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
	srv.BroadcastChan <- broadcastMessage
}

func (srv *Server) ListenForBroadcast() {
	go srv.run()
}

func (srv *Server) run() {
	for msg := range srv.BroadcastChan {
		*srv.Round = msg.GetRound()
		//srv.c.StoreID(msgID-1)
		req := msg.GetRequest()
		method := msg.GetMethod()
		ctx := context.Background()
		// if another function is called in broadcast, the request needs to be converted
		//if convertFunc, ok := srv.conversions[method]; ok {
		//	convertedReq := convertFunc(ctx, req)
		//	srv.methods[method](ctx, convertedReq)
		//	continue
		//}
		srv.methods[method](ctx, req)
	}
}

func RegisterBroadcastFunc[T requestTypes, V responseTypes](impl func(context.Context, T) (V, error)) func(context.Context, requestTypes) (responseTypes, error) {
	return func(ctx context.Context, req requestTypes) (resp responseTypes, err error) {
		return impl(ctx, req.(T))
	}
}

func RegisterConversionFunc[T requestTypes, V responseTypes](impl func(context.Context, T) V) func(context.Context, any) any {
	return func(ctx context.Context, req any) any {
		return impl(ctx, req.(T))
	}
}

//func (srv *Server) RegisterConversion(method string, conversion func(context.Context, any) any) {
//	srv.conversions[method] = conversion
//}

func (srv *Server) RegisterBroadcastFunc(method string, broadcastFunc func(context.Context, requestTypes) (responseTypes, error)) {
	srv.methods[method] = broadcastFunc
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
