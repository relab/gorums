package gorums

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/relab/gorums/authentication"
	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/logging"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ReplySpecHandler func(req protoreflect.ProtoMessage, replies []protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)

type csr struct {
	ctx      context.Context
	cancel   context.CancelFunc
	respChan chan protoreflect.ProtoMessage
}

type ClientServer struct {
	id         uint64 // should correspond to the MachineID given to the manager
	addr       string
	mu         sync.Mutex
	csr        map[uint64]*csr
	lis        net.Listener
	ctx        context.Context
	cancelCtx  context.CancelFunc
	grpcServer *grpc.Server
	handlers   map[string]requestHandler
	logger     *slog.Logger
	auth       *authentication.EllipticCurve
	allowList  map[string]string
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
		OriginAddr:        srv.addr,
	}
	// we expect one response when we are done
	doneChan := make(chan protoreflect.ProtoMessage, 1)
	// we should buffer this channel according to the number of servers.
	// most configs hopefully contain less than 7 servers.
	respChan := make(chan protoreflect.ProtoMessage, 7)
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
		logger = srv.logger.With(logging.BroadcastID(broadcastID))
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
				logger.Warn("clientserver: stopped by client", logging.Cancelled(true))
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
				logger.Warn("clientserver: stopped by server", logging.Cancelled(true))
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
						logger.Info("clientserver: req done", logging.Cancelled(false))
					}
				case <-ctx.Done():
					if logger != nil {
						logger.Warn("clientserver: req done but stopped by server", logging.Cancelled(true))
					}
				case <-clientCtx.Done():
					if logger != nil {
						logger.Warn("clientserver: req done but cancelled by client", logging.Cancelled(true))
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
		srv.logger.Info("clientserver: got a reply", logging.BroadcastID(broadcastID))
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
		err = s.verify(req)
		if err != nil {
			continue
		}
		if handler, ok := s.handlers[req.Metadata.GetMethod()]; ok {
			go handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &mut}, req, nil)
			mut.Lock()
		}
	}
}

// NewClientServer returns a new instance of ClientServer.
// This function is intended for internal Gorums use.
// You should call `NewServer` in the generated code instead.
func NewClientServer(lis net.Listener, opts ...ServerOption) *ClientServer {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	if serverOpts.listenAddr == "" {
		panic("The listen addr cannot be empty. Provide the WithListenAddr() option to AddClientServer().")
	}
	var logger *slog.Logger
	if serverOpts.logger != nil {
		logger = serverOpts.logger.With(logging.MachineID(serverOpts.machineID))
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &ClientServer{
		id:         serverOpts.machineID,
		addr:       serverOpts.listenAddr,
		ctx:        ctx,
		cancelCtx:  cancel,
		csr:        make(map[uint64]*csr),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
		handlers:   make(map[string]requestHandler),
		logger:     logger,
	}
	ordering.RegisterGorumsServer(srv.grpcServer, srv)
	srv.lis = lis
	if serverOpts.auth != nil {
		srv.auth = serverOpts.auth
	}
	if serverOpts.allowList != nil {
		srv.allowList = serverOpts.allowList
	}
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
	if srv.addr == "" {
		srv.addr = listener.Addr().String()
	}
	return srv.grpcServer.Serve(listener)
}

func (srv *ClientServer) verify(req *Message) error {
	if srv.auth == nil {
		return nil
	}
	if req.Metadata.GetAuthMsg() == nil {
		return fmt.Errorf("missing authMsg")
	}
	if req.Metadata.GetAuthMsg().GetSignature() == nil {
		return fmt.Errorf("missing signature")
	}
	if req.Metadata.GetAuthMsg().GetPublicKey() == "" {
		return fmt.Errorf("missing publicKey")
	}
	authMsg := req.Metadata.GetAuthMsg()
	if srv.allowList != nil {
		pemEncodedPub, ok := srv.allowList[authMsg.GetSender()]
		if !ok {
			return fmt.Errorf("not allowed")
		}
		if pemEncodedPub != authMsg.GetPublicKey() {
			return fmt.Errorf("publicKey did not match")
		}
	}
	valid, err := srv.auth.VerifySignature(authMsg.GetPublicKey(), req.Encode(), authMsg.GetSignature())
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func createClient(addr string, dialOpts []grpc.DialOption) (*broadcast.Client, error) {
	// necessary to ensure correct marshalling and unmarshaling of gorums messages
	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.CallContentSubtype(ContentSubtype)))
	opts := newManagerOptions()
	opts.grpcDialOpts = dialOpts
	mgr := &RawManager{
		opts: opts,
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
		SendMsg: func(broadcastID uint64, method string, msg protoreflect.ProtoMessage, timeout time.Duration, originDigest, originSignature []byte, originPubKey string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cd := CallData{
				Method:          method,
				Message:         msg,
				BroadcastID:     broadcastID,
				OriginDigest:    originDigest,
				OriginSignature: originSignature,
				OriginPubKey:    originPubKey,
			}
			_, err := node.RPCCall(ctx, cd)
			return err
		},
		Close: func() error {
			mgr.Close()
			node.close()
			return nil
		},
	}, nil
}
