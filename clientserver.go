package gorums

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ReplySpecHandler func([]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)

type ClientResponse struct {
	broadcastID string
	data        protoreflect.ProtoMessage
}

type ClientRequest struct {
	broadcastID string
	doneChan    chan protoreflect.ProtoMessage
	handler     ReplySpecHandler
}

type csr struct {
	resps    []protoreflect.ProtoMessage
	doneChan chan protoreflect.ProtoMessage
	handler  func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
}

type ClientServer struct {
	mu        sync.Mutex
	csr       map[uint64]*csr
	respChan  chan *ClientResponse
	reqChan   chan *ClientRequest
	resps     map[string][]protoreflect.ProtoMessage
	doneChans map[string]chan protoreflect.ProtoMessage
	handlers  map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
	lis       net.Listener
	ctx       context.Context
	cancelCtx context.CancelFunc
}

func NewClientServer(lis net.Listener) (*ClientServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &ClientServer{
		respChan:  make(chan *ClientResponse, 10),
		reqChan:   make(chan *ClientRequest),
		resps:     make(map[string][]protoreflect.ProtoMessage),
		doneChans: make(map[string]chan protoreflect.ProtoMessage),
		handlers:  make(map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)),
		csr:       make(map[uint64]*csr),
		ctx:       ctx,
		cancelCtx: cancel,
	}
	srv.lis = lis
	//go srv.handle()
	return srv, nil
}

func (srv *ClientServer) Stop() {
	srv.cancelCtx()
}

func (srv *ClientServer) AddRequest(broadcastID uint64, ctx context.Context, in protoreflect.ProtoMessage, handler ReplySpecHandler, method string) (chan protoreflect.ProtoMessage, QuorumCallData) {
	cd := QuorumCallData{
		Message: in,
		Method:  method,

		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        srv.lis.Addr().String(),
	}
	doneChan := make(chan protoreflect.ProtoMessage)

	srv.mu.Lock()
	srv.csr[broadcastID] = &csr{
		resps:    make([]protoreflect.ProtoMessage, 0, 3),
		doneChan: doneChan,
		handler:  handler,
	}
	//srv.resps[broadcastID] = make([]protoreflect.ProtoMessage, 0)
	//srv.doneChans[broadcastID] = doneChan
	//srv.handlers[broadcastID] = handler
	srv.mu.Unlock()
	//srv.reqChan <- &ClientRequest{
	//broadcastID: broadcastID,
	//doneChan:    doneChan,
	//handler:     handler,
	//}
	return doneChan, cd
}

func (srv *ClientServer) AddResponse(ctx context.Context, resp protoreflect.ProtoMessage) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("no metadata")
	}
	broadcastID := uint64(0)
	val := md.Get(BroadcastID)
	if val != nil && len(val) >= 1 {
		bID, err := strconv.Atoi(val[0])
		broadcastID = uint64(bID)
		if err != nil {
			return err
		}
	}
	if broadcastID == 0 {
		return fmt.Errorf("no broadcastID")
	}

	srv.mu.Lock()
	csr, ok := srv.csr[broadcastID]
	if !ok {
		srv.mu.Unlock()
		return fmt.Errorf("doesn't exist")
	}
	csr.resps = append(csr.resps, resp)
	response, done := csr.handler(csr.resps)
	if done {
		csr.doneChan <- response
		delete(srv.csr, broadcastID)
	}
	srv.mu.Unlock()
	return nil
}

func ConvertToType[T protoreflect.ProtoMessage](handler func([]T) (T, bool)) func(d []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
	return func(d []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		data := make([]T, len(d))
		for i, elem := range d {
			data[i] = elem.(T)
		}
		return handler(data)
	}
}

func ServerClientRPC(method string) func(broadcastID uint64, in protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error) {
	return func(broadcastID uint64, in protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error) {
		tmp := strings.Split(method, ".")
		m := ""
		if len(tmp) >= 1 {
			m = tmp[len(tmp)-1]
		}
		clientMethod := "/protos.ClientServer/Client" + m
		out := new(any)
		md := metadata.Pairs(BroadcastID, strconv.Itoa(int(broadcastID)))
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		ctx = metadata.NewOutgoingContext(ctx, md)
		err := cc.Invoke(ctx, clientMethod, in, out, opts...)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}
