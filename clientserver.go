package gorums

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

type ClientServer struct {
	respChan   chan *ClientResponse
	reqChan    chan *ClientRequest
	resps      map[string][]protoreflect.ProtoMessage
	doneChans  map[string]chan protoreflect.ProtoMessage
	handlers   map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
	listenAddr string
}

func NewClientServer(lis net.Listener) (*ClientServer, error) {
	srv := &ClientServer{
		respChan:  make(chan *ClientResponse, 10),
		reqChan:   make(chan *ClientRequest),
		resps:     make(map[string][]protoreflect.ProtoMessage),
		doneChans: make(map[string]chan protoreflect.ProtoMessage),
		handlers:  make(map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)),
	}
	srv.listenAddr = lis.Addr().String()
	go srv.handle()
	return srv, nil
}

func (srv *ClientServer) AddRequest(ctx context.Context, in protoreflect.ProtoMessage, handler ReplySpecHandler) (chan protoreflect.ProtoMessage, QuorumCallData) {
	broadcastID := uuid.New().String()
	cd := QuorumCallData{
		Message: in,
		Method:  "protos.UniformBroadcast.SaveStudent",

		BroadcastID: broadcastID,
		SenderType:  BroadcastClient,
		OriginAddr:  srv.listenAddr,
	}
	doneChan := make(chan protoreflect.ProtoMessage)
	srv.reqChan <- &ClientRequest{
		broadcastID: broadcastID,
		doneChan:    doneChan,
		handler:     handler,
	}
	return doneChan, cd
}

func (srv *ClientServer) AddResponse(ctx context.Context, resp protoreflect.ProtoMessage) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return fmt.Errorf("no metadata")
	}
	broadcastID := ""
	val := md.Get(BroadcastID)
	if val != nil && len(val) >= 1 {
		broadcastID = val[0]
	}
	if broadcastID == "" {
		return fmt.Errorf("no broadcastID")
	}
	srv.respChan <- &ClientResponse{
		broadcastID: broadcastID,
		data:        resp,
	}
	return nil
}

func (srv *ClientServer) handle() {
	for {
		select {
		case resp := <-srv.respChan:
			if _, ok := srv.resps[resp.broadcastID]; !ok {
				continue
			}
			srv.resps[resp.broadcastID] = append(srv.resps[resp.broadcastID], resp.data)
			response, done := srv.handlers[resp.broadcastID](srv.resps[resp.broadcastID])
			if done {
				srv.doneChans[resp.broadcastID] <- response
				close(srv.doneChans[resp.broadcastID])
				delete(srv.resps, resp.broadcastID)
				delete(srv.doneChans, resp.broadcastID)
				delete(srv.handlers, resp.broadcastID)
			}
		case req := <-srv.reqChan:
			srv.resps[req.broadcastID] = make([]protoreflect.ProtoMessage, 0)
			srv.doneChans[req.broadcastID] = req.doneChan
			srv.handlers[req.broadcastID] = req.handler
		}
	}
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

func ServerClientRPC(method string) func(addr, broadcastID string, in protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error) {
	return func(addr, broadcastID string, in protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error) {
		tmp := strings.Split(method, ".")
		m := ""
		if len(tmp) >= 1 {
			m = tmp[len(tmp)-1]
		}
		clientMethod := "/protos.ClientServer/Client" + m
		cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		out := new(any)
		md := metadata.Pairs(BroadcastID, broadcastID)
		ctx := metadata.NewOutgoingContext(context.Background(), md)
		err = cc.Invoke(ctx, clientMethod, in, out, opts...)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}
