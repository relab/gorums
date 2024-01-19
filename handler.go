package gorums

import (
	"context"
	"fmt"
	reflect "reflect"
	"sync/atomic"

	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type requestTypes interface {
	ProtoReflect() protoreflect.Message
}

type responseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastFunc func(ctx context.Context, req any) (resp any, err error)
type ConversionFunc func(ctx context.Context, req any) any

type defaultImplementationFunc[T requestTypes, V responseTypes] func(ServerCtx, T) (V, error)
type implementationFunc[T requestTypes, V responseTypes] func(ServerCtx, T) (V, error, bool)

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

func newBroadcastMessage[T requestTypes, V responseTypes](ctx ServerCtx, req T, impl implementationFunc[T, V], method string, round uint64) *broadcastMessage[T, V] {
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	return &broadcastMessage[T, V]{
		from:           addr,
		request:        req,
		implementation: impl,
		method:         method,
		context:        ctx,
		round:          round,
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
		fmt.Println("sender:", in.Metadata.Sender, in.Metadata.Method, "round:", in.Metadata.Round)
		resp, err, broadcast := impl(ctx, req)
		if broadcast && !srv.alreadyBroadcasted(in.Metadata.Round, in.Metadata.Method) {
			go srv.broadcast(newBroadcastMessage[T, V](ctx, req, impl, in.Metadata.Method, in.Metadata.Round))
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

func (srv *Server) alreadyReceivedFromPeer(ctx ServerCtx, msgId, round uint64, method, sender string) bool {
	_, ok := srv.recievedFrom[msgId]
	if !ok {
		srv.recievedFrom[msgId] = make(map[string]map[string]bool)
	}
	_, ok = srv.recievedFrom[msgId][method]
	if !ok {
		srv.recievedFrom[msgId][method] = make(map[string]bool)
	}
	p, _ := peer.FromContext(ctx)
	addr := p.Addr.String()
	receivedMsgFromNode, ok := srv.recievedFrom[msgId][method][addr]
	if !ok && !receivedMsgFromNode {
		srv.recievedFrom[msgId][method][addr] = true
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
		if convertFunc, ok := srv.conversions[method]; ok {
			convertedReq := convertFunc(ctx, req)
			srv.methods[method](ctx, convertedReq)
			continue
		}
		srv.methods[method](ctx, req)
	}
}

func RegisterBroadcastFunc[T requestTypes, V responseTypes](impl func(context.Context, T) (V, error)) func(context.Context, any) (any, error) {
	return func(ctx context.Context, req any) (resp any, err error) {
		return impl(ctx, req.(T))
	}
}

func RegisterConversionFunc[T requestTypes, V responseTypes](impl func(context.Context, T) V) func(context.Context, any) any {
	return func(ctx context.Context, req any) any {
		return impl(ctx, req.(T))
	}
}

func (srv *Server) RegisterConversion(method string, conversion func(context.Context, any) any) {
	srv.conversions[method] = conversion
}

func (srv *Server) RegisterBroadcastFunc(method string, broadcastFunc func(context.Context, any) (any, error)) {
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
