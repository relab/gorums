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

type ReplySpecHandler func(req protoreflect.ProtoMessage, replies []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)

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
	req      protoreflect.ProtoMessage
	resps    []protoreflect.ProtoMessage
	doneChan chan protoreflect.ProtoMessage
	handler  ReplySpecHandler
}

type ClientServer struct {
	mu        sync.Mutex
	csr       map[uint64]*csr
	reqChan   chan *ClientRequest
	lis       net.Listener
	ctx       context.Context
	cancelCtx context.CancelFunc
}

func NewClientServer(lis net.Listener) (*ClientServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &ClientServer{
		csr:       make(map[uint64]*csr),
		ctx:       ctx,
		cancelCtx: cancel,
	}
	srv.lis = lis
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
		req:      in,
		resps:    make([]protoreflect.ProtoMessage, 0, 3),
		doneChan: doneChan,
		handler:  handler,
	}
	srv.mu.Unlock()
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
	response, done := csr.handler(csr.req, csr.resps)
	if done {
		csr.doneChan <- response
		delete(srv.csr, broadcastID)
	}
	srv.mu.Unlock()
	return nil
}

func ConvertToType[T, U protoreflect.ProtoMessage](handler func(U, []T) (T, bool)) ReplySpecHandler {
	return func(req protoreflect.ProtoMessage, replies []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		data := make([]T, len(replies))
		for i, elem := range replies {
			data[i] = elem.(T)
		}
		return handler(req.(U), data)
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
