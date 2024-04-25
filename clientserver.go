package gorums

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func init() {
	if encoding.GetCodec(ContentSubtype) == nil {
		encoding.RegisterCodec(NewCodec())
	}
}

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

func (c *ClientServer) status() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(5 * time.Second):
			fmt.Println(c.inProgress)
		}
	}
}

type ClientServer struct {
	mu         sync.Mutex
	csr        map[uint64]*csr
	reqChan    chan *ClientRequest
	lis        net.Listener
	ctx        context.Context
	cancelCtx  context.CancelFunc
	inProgress uint64
	grpcServer *grpc.Server
	handlers   map[string]requestHandler
	ordering.UnimplementedGorumsServer
}

func NewClientServer(lis net.Listener) (*ClientServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &ClientServer{
		csr:       make(map[uint64]*csr),
		ctx:       ctx,
		cancelCtx: cancel,
	}
	srv.lis = lis
	go srv.status()
	return srv, nil
}

func (srv *ClientServer) Stop() {
	if srv.cancelCtx != nil {
		srv.cancelCtx()
	}
	if srv.grpcServer != nil {
		srv.grpcServer.Stop()
	}
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
	srv.inProgress++
	srv.mu.Unlock()
	return doneChan, cd
}

func (srv *ClientServer) AddResponse(ctx context.Context, resp protoreflect.ProtoMessage, broadcastID uint64) error {
	//md, ok := metadata.FromIncomingContext(ctx)
	//if !ok {
	//return fmt.Errorf("no metadata")
	//}
	//broadcastID := uint64(0)
	//val := md.Get(BroadcastID)
	//if val != nil && len(val) >= 1 {
	//bID, err := strconv.Atoi(val[0])
	//broadcastID = uint64(bID)
	//if err != nil {
	//return err
	//}
	//}
	if broadcastID == 0 {
		return fmt.Errorf("no broadcastID")
	}

	//slog.Info("AddResponse: Lock", "broadcastID", broadcastID)
	srv.mu.Lock()
	//slog.Info("AddResponse: Inside", "broadcastID", broadcastID)
	csr, ok := srv.csr[broadcastID]
	if !ok {
		srv.mu.Unlock()
		return fmt.Errorf("doesn't exist")
	}
	csr.resps = append(csr.resps, resp)
	response, done := csr.handler(csr.req, csr.resps)

	var doneChan chan protoreflect.ProtoMessage
	if done {
		srv.inProgress--
		doneChan = csr.doneChan
		delete(srv.csr, broadcastID)
		srv.mu.Unlock()
		//slog.Info("AddResponse: Unlock", "broadcastID", broadcastID)
		doneChan <- response
		//slog.Info("add response", "response", resp, "response", response)
		//slog.Info("AddResponse: Done, sent", "resp", response, "broadcastID", broadcastID)
		return nil
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
			//_, ok := in.(protoreflect.ProtoMessage)
			//slog.Error("clientserver: ServerClientRPC", "err", err, "req", in, "proto.Message", ok)
			return nil, err
		}
		return nil, nil
	}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *ClientServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	ctx := srv.Context()
	for {
		req := newMessage(responseType)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[req.Metadata.Method]; ok {
			// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
			// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
			// finished, or when it is safe to start processing the next request.
			go handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: nil}, req, nil)
		}
	}
}

// NewServer returns a new instance of GorumsServer.
// This function is intended for internal Gorums use.
// You should call `NewServer` in the generated code instead.
func NewClientServer2(lis net.Listener, opts ...ServerOption) *ClientServer {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	srv := &ClientServer{
		csr:        make(map[uint64]*csr),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
		handlers:   make(map[string]requestHandler),
	}
	ordering.RegisterGorumsServer(srv.grpcServer, srv)
	srv.lis = lis
	return srv
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (srv *ClientServer) RegisterHandler(method string, handler requestHandler) {
	srv.handlers[method] = handler
}

// Serve starts serving on the listener.
func (srv *ClientServer) Serve(listener net.Listener) error {
	return srv.grpcServer.Serve(listener)
}

func createClient(addr string, dialOpts []grpc.DialOption) (*broadcast.Client, error) {
	mgr := &RawManager{
		opts: managerOptions{
			grpcDialOpts: dialOpts,
		},
	}
	node, err := NewRawNode(addr)
	if err != nil {
		return nil, err
	}
	err = node.connect(mgr)
	if err != nil {
		return nil, err
	}
	return &broadcast.Client{
		Addr: node.Address(),
		SendMsg: func(broadcastID uint64, method string, msg protoreflect.ProtoMessage, timeout time.Duration) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cd := CallData{
				Method:      method,
				Message:     msg,
				BroadcastID: broadcastID,
			}
			node.RPCCall(ctx, cd)
		},
	}, nil
}
