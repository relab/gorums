package gorums

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

//func init() {
//if encoding.GetCodec(ContentSubtype) == nil {
//encoding.RegisterCodec(NewCodec())
//}
//}

type ReplySpecHandler func(req protoreflect.ProtoMessage, replies []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)

type ClientResponse struct {
	err error
	msg protoreflect.ProtoMessage
}

type ClientRequest struct {
	broadcastID string
	doneChan    chan protoreflect.ProtoMessage
	handler     ReplySpecHandler
}

type csr struct {
	ctx      context.Context
	cancel   context.CancelFunc
	req      protoreflect.ProtoMessage
	resps    []protoreflect.ProtoMessage
	doneChan chan protoreflect.ProtoMessage
	respChan chan protoreflect.ProtoMessage
	handler  ReplySpecHandler
}

type ClientServer struct {
	id         uint64 // should correpond to the ID given to the manager
	mu         sync.Mutex
	csr        map[uint64]*csr
	reqChan    chan *ClientRequest
	lis        net.Listener
	ctx        context.Context
	cancelCtx  context.CancelFunc
	inProgress uint64
	grpcServer *grpc.Server
	handlers   map[string]requestHandler
	logger     *slog.Logger
	ordering.UnimplementedGorumsServer
}

func (srv *ClientServer) Stop() {
	if srv.logger != nil {
		srv.logger.Info("clientserver: stopped")
	}
	if srv.cancelCtx != nil {
		srv.cancelCtx()
	}
	if srv.grpcServer != nil {
		srv.grpcServer.Stop()
	}
}

func (srv *ClientServer) AddRequest(broadcastID uint64, clientCtx context.Context, in protoreflect.ProtoMessage, handler ReplySpecHandler, method string) (chan protoreflect.ProtoMessage, BroadcastCallData) {
	cd := BroadcastCallData{
		Message: in,
		Method:  method,

		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        srv.lis.Addr().String(),
	}
	doneChan := make(chan protoreflect.ProtoMessage)
	respChan := make(chan protoreflect.ProtoMessage)
	ctx, cancel := context.WithCancel(srv.ctx)

	srv.mu.Lock()
	srv.csr[broadcastID] = &csr{
		ctx:      ctx,
		cancel:   cancel,
		respChan: respChan,
	}
	srv.mu.Unlock()

	var logger *slog.Logger
	if srv.logger != nil {
		logger = srv.logger.With(slog.Uint64("BroadcastID", broadcastID))
	}
	go createReq(ctx, clientCtx, cancel, in, doneChan, respChan, handler, logger)

	return doneChan, cd
}

func createReq(ctx, clientCtx context.Context, cancel context.CancelFunc, req protoreflect.ProtoMessage, doneChan chan protoreflect.ProtoMessage, respChan chan protoreflect.ProtoMessage, handler ReplySpecHandler, logger *slog.Logger) {
	// make sure to cancel the req ctx when returning to
	// prevent a leaking ctx.
	defer cancel()
	resps := make([]protoreflect.ProtoMessage, 0, 3)
	for {
		select {
		case <-clientCtx.Done():
			// client provided ctx
			if logger != nil {
				logger.Warn("clientserver: stopped by client", "cancelled", true)
			}
			return
		case <-ctx.Done():
			// request ctx. this is a child to the server ctx.
			// hence, guaranteeing that all reqs are finished
			// when the server is stopped.

			// we must send on channel to prevent deadlock on
			// the receiving end. this can happen if the client
			// chooses not to timeout the request and the server
			// goes down.
			close(doneChan)
			if logger != nil {
				logger.Warn("clientserver: stopped by server", "cancelled", true)
			}
			return
		case resp := <-respChan:
			// keep track of all responses thus far
			resps = append(resps, resp)
			// handler is the QSpec method provided by the implementer.
			response, done := handler(req, resps)
			if done {
				select {
				case doneChan <- response:
					if logger != nil {
						logger.Info("clientserver: req done", "cancelled", false)
					}
				case <-ctx.Done():
					if logger != nil {
						logger.Warn("clientserver: req done but stopped by server", "cancelled", true)
					}
				case <-clientCtx.Done():
					if logger != nil {
						logger.Warn("clientserver: req done but cancelled by client", "cancelled", true)
					}
				}
				close(doneChan)
				return
			}
		}
	}

}

func (srv *ClientServer) AddResponse(ctx context.Context, resp protoreflect.ProtoMessage, broadcastID uint64) error {
	if broadcastID == 0 {
		return fmt.Errorf("no broadcastID")
	}
	srv.mu.Lock()
	csr, ok := srv.csr[broadcastID]
	srv.mu.Unlock()

	if !ok {
		return fmt.Errorf("doesn't exist")
	}
	if srv.logger != nil {
		srv.logger.Info("clientserver: got a reply", "BroadcastID", broadcastID)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-csr.ctx.Done():
		return csr.ctx.Err()
	case csr.respChan <- resp:
	}
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

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *ClientServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	//slog.Info("clientserver: connected to client", "addr", s.lis.Addr().String())
	var mut sync.Mutex // used to achieve mutex between request handlers
	ctx := srv.Context()
	// Start with a locked mutex
	mut.Lock()
	defer mut.Unlock()
	for {
		req := newMessage(responseType)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[req.Metadata.Method]; ok {
			go handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &mut}, req, nil)
			mut.Lock()
		}
	}
}

// NewServer returns a new instance of GorumsServer.
// This function is intended for internal Gorums use.
// You should call `NewServer` in the generated code instead.
func NewClientServer(lis net.Listener, opts ...ServerOption) *ClientServer {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	var logger *slog.Logger
	if serverOpts.logger != nil {
		logger = serverOpts.logger.With(slog.Uint64("ClientID", serverOpts.machineID))
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &ClientServer{
		id:         serverOpts.machineID,
		ctx:        ctx,
		cancelCtx:  cancel,
		csr:        make(map[uint64]*csr),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
		handlers:   make(map[string]requestHandler),
		logger:     logger,
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
	// necessary to ensure correct marshalling and unmarshalling of gorums messages
	// TODO: find a better solution
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.CallContentSubtype(ContentSubtype)))
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
		SendMsg: func(broadcastID uint64, method string, msg protoreflect.ProtoMessage, timeout time.Duration) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cd := CallData{
				Method:      method,
				Message:     msg,
				BroadcastID: broadcastID,
			}
			node.Unicast(ctx, cd)
			return node.LastErr()
		},
		Close: func() error {
			return node.close()
		},
	}, nil
}
