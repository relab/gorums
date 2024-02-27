package dev

import (
	context "context"
	"net"
	"strings"

	"github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

type Server struct {
	*gorums.Server
}

func NewServer() *Server {
	srv := &Server{
		gorums.NewServer(),
	}
	b := &Broadcast{
		BroadcastStruct: gorums.NewBroadcastStruct(),
		sp:              gorums.NewSpBroadcastStruct(),
	}
	srv.RegisterBroadcastStruct(b, configureHandlers(b), configureMetadata(b))
	return srv
}

func (srv *Server) SetView(ownAddr string, srvAddrs []string, opts ...gorums.ManagerOption) error {
	err := srv.RegisterView(ownAddr, srvAddrs, opts...)
	srv.ListenForBroadcast()
	return err
}

type Broadcast struct {
	*gorums.BroadcastStruct
	sp       *gorums.SpBroadcast
	metadata gorums.BroadcastMetadata
}

func configureHandlers(b *Broadcast) func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
	return func(bh gorums.BroadcastHandlerFunc, ch gorums.BroadcastReturnToClientHandlerFunc) {
		b.sp.BroadcastHandler = bh
		b.sp.ReturnToClientHandler = ch
	}
}

func configureMetadata(b *Broadcast) func(metadata gorums.BroadcastMetadata) {
	return func(metadata gorums.BroadcastMetadata) {
		b.metadata = metadata
	}
}

// Returns a readonly struct of the metadata used in the broadcast.
//
// Note: Some of the data are equal across the cluster, such as BroadcastID.
// Other fields are local, such as SenderAddr.
func (b *Broadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}

type clientResponse struct {
	broadcastID string
	data        protoreflect.ProtoMessage
}

type clientRequest struct {
	broadcastID string
	doneChan    chan protoreflect.ProtoMessage
	handler     func([]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error)
}

type clientServerImpl struct {
	grpcServer *grpc.Server
	respChan   chan *clientResponse
	reqChan    chan *clientRequest
	resps      map[string][]protoreflect.ProtoMessage
	doneChans  map[string]chan protoreflect.ProtoMessage
	handlers   map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error)
}

func (c *Configuration) RegisterClientServer(listenAddr string, replySpec ReplySpec) {
	var opts []grpc.ServerOption
	srv := &clientServerImpl{
		grpcServer: grpc.NewServer(opts...),
		respChan:   make(chan *clientResponse, 10),
		reqChan:    make(chan *clientRequest),
		resps:      make(map[string][]protoreflect.ProtoMessage),
		doneChans:  make(map[string]chan protoreflect.ProtoMessage),
		handlers:   make(map[string]func(resps []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error)),
	}
	lis, err := net.Listen("tcp", listenAddr)
	for err != nil {
		return
	}
	c.listenAddr = lis.Addr().String()
	srv.grpcServer.RegisterService(&clientServer_ServiceDesc, srv)
	go srv.grpcServer.Serve(lis)
	go srv.handle()
	c.srv = srv
	c.replySpec = replySpec
}

func (srv *clientServerImpl) handle() {
	for {
		select {
		case resp := <-srv.respChan:
			if _, ok := srv.resps[resp.broadcastID]; !ok {
				continue
			}
			srv.resps[resp.broadcastID] = append(srv.resps[resp.broadcastID], resp.data)
			response, err := srv.handlers[resp.broadcastID](srv.resps[resp.broadcastID])
			if err == nil {
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

func convertToType[T protoreflect.ProtoMessage](handler func([]T) (T, error)) func(d []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error) {
	return func(d []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, error) {
		data := make([]T, len(d))
		for i, elem := range d {
			data[i] = elem.(T)
		}
		return handler(data)
	}
}

func _serverClientRPC(method string) func(addr, broadcastID string, in protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error) {
	return func(addr, broadcastID string, in protoreflect.ProtoMessage, opts ...grpc.CallOption) (any, error) {
		tmp := strings.Split(method, ".")
		m := ""
		if len(tmp) >= 1 {
			m = tmp[len(tmp)-1]
		}
		method = "protos.ClientServer." + m
		cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		out := new(any)
		md := metadata.Pairs("broadcastID", broadcastID)
		ctx := metadata.NewOutgoingContext(context.Background(), md)
		err = cc.Invoke(ctx, method, in, out, opts...)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}
