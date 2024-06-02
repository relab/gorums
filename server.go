package gorums

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// requestHandler is used to fetch a response message based on the request.
// A requestHandler should receive a message from the server, unmarshal it into
// the proper type for that Method's request type, call a user provided Handler,
// and return a marshaled result to the server.
type requestHandler func(ServerCtx, *Message, chan<- *Message)

type orderingServer struct {
	handlers map[string]requestHandler
	opts     *serverOptions
	ordering.UnimplementedGorumsServer
}

func newOrderingServer(opts *serverOptions) *orderingServer {
	s := &orderingServer{
		handlers: make(map[string]requestHandler),
		opts:     opts,
	}
	return s
}

// SendMessage attempts to send a message on a channel.
//
// This function should be used by generated code only.
func SendMessage(ctx context.Context, c chan<- *Message, msg *Message) error {
	select {
	case c <- msg:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// WrapMessage wraps the metadata, response and error status in a gorumsMessage
//
// This function should be used by generated code only.
func WrapMessage(md *ordering.Metadata, resp protoreflect.ProtoMessage, err error) *Message {
	errStatus, ok := status.FromError(err)
	if !ok {
		errStatus = status.New(codes.Unknown, err.Error())
	}
	md.Status = errStatus.Proto()
	return &Message{Metadata: md, Message: resp}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *Message, s.opts.buffer)
	ctx := srv.Context()
	//md, _ := metadata.FromIncomingContext(ctx)
	//slog.Error("NodeStream created", "publicKey", md.Get("publicKey"))

	if s.opts.connectCallback != nil {
		s.opts.connectCallback(ctx)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-finished:
				err := srv.SendMsg(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	// Start with a locked mutex
	mut.Lock()
	defer mut.Unlock()

	for {
		req := newMessage(requestType)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[req.Metadata.Method]; ok {
			// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
			// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
			// finished, or when it is safe to start processing the next request.
			go handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &mut}, req, finished)
			// Wait until the handler releases the mutex.
			mut.Lock()
		}
	}
}

type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	logger          *slog.Logger
	executionOrder  map[string]int
	machineID       uint64
	// this is the address other nodes should connect to. Sometimes, e.g. when
	// running in a docker container it is useful to listen to the loopback
	// address and use forwarding from the host. If not this option is not given,
	// the listen address used on the gRPC listener will be used instead.
	listenAddr string
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

// WithReceiveBufferSize sets the buffer size for the server.
// A larger buffer may result in higher throughput at the cost of higher latency.
func WithReceiveBufferSize(size uint) ServerOption {
	return func(o *serverOptions) {
		o.buffer = size
	}
}

// WithGRPCServerOptions allows to set gRPC options for the server.
func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.grpcOpts = append(o.grpcOpts, opts...)
	}
}

// WithConnectCallback registers a callback function that will be called by the server
// whenever a node connects or reconnects to the server. This allows access to the node's
// stream context, which is passed to the callback function. The stream context can be
// used to extract the metadata and peer information, if available.
func WithConnectCallback(callback func(context.Context)) ServerOption {
	return func(so *serverOptions) {
		so.connectCallback = callback
	}
}

// WithOrder returns a ServerOption which defines the order of execution
// of gRPC methods.
//
// E.g. in PBFT we can specify the order: PrePrepare, Prepare, and Commit.
// Gorums will then make sure to only execute messages in this order.
// The rules are defined as such:
//  1. Messages to the first gRPC method (PrePrepare) will always be executed.
//  2. Messages to gRPC methods not defined in the order will always be executed.
//  3. Messages to gRPC methods appearing later in the order will be cached and executed later.
//  4. Messages to gRPC methods appearing earlier than the current method will be executed immediately.
//     E.g. if current method is Commit, then messages to PrePrepare and Prepare will be accepted.
//  5. The server itself needs to call the next method in the order to progress to the next gRPC method.
//     E.g. by calling broadcast.Prepare().
//  6. If the BroadcastOption ProgressTo() is used, then it will progress to the given gRPC method.
func WithOrder(executionOrder ...string) ServerOption {
	return func(o *serverOptions) {
		o.executionOrder = make(map[string]int)
		for i, method := range executionOrder {
			o.executionOrder[method] = i
		}
	}
}

// WithSLogger returns a ServerOption which sets an optional structured logger for
// the Server. This will log internal events regarding broadcast requests. The
// ManagerOption WithLogger() should be used when creating the manager in order
// to log events related to transmission of messages.
func WithSLogger(logger *slog.Logger) ServerOption {
	return func(o *serverOptions) {
		o.logger = logger
	}
}

// WithSrvID sets the MachineID of the broadcast server. This ID is used to
// generate BroadcastIDs. This method should be used if a replica needs to
// initiate a broadcast request.
//
// An example use case is in Paxos:
// The designated leader sends a prepare and receives some promises it has
// never seen before. It thus needs to send accept messages correspondingly.
// These accept messages are not part of any broadcast request and the server
// is thus responsible for the origin of these requests.
func WithSrvID(machineID uint64) ServerOption {
	return func(o *serverOptions) {
		o.machineID = machineID
	}
}

// WithListenAddr sets the IP address of the broadcast server which will be used in messages
// sent by the server. The network of the address has to be a TCP network name. Hence,
// net.ResolveTCPAddr() can be used to obtain the net.Addr type of an address.
func WithListenAddr(listenAddr net.Addr) ServerOption {
	return func(o *serverOptions) {
		o.listenAddr = listenAddr.String()
	}
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv          *orderingServer
	grpcServer   *grpc.Server
	broadcastSrv *broadcastServer
}

// NewServer returns a new instance of GorumsServer.
// This function is intended for internal Gorums use.
// You should call `NewServer` in the generated code instead.
func NewServer(opts ...ServerOption) *Server {
	serverOpts := serverOptions{
		// Provide an illegal machineID to avoid unintentional collisions.
		// 0 is a valid MachineID and should not be used as default.
		machineID: uint64(broadcast.MaxMachineID) + 1,
	}
	for _, opt := range opts {
		opt(&serverOpts)
	}
	s := &Server{
		srv:          newOrderingServer(&serverOpts),
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		broadcastSrv: newBroadcastServer(&serverOpts),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler requestHandler) {
	s.broadcastSrv.registerBroadcastFunc(method)
	s.srv.handlers[method] = handler
}

func (s *Server) RegisterClientHandler(method string) {
	s.broadcastSrv.registerSendToClientHandler(method)
}

// Serve starts serving on the listener.
func (s *Server) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *Server) GracefulStop() {
	if s.broadcastSrv != nil {
		s.broadcastSrv.stop()
	}
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately.
func (s *Server) Stop() {
	if s.broadcastSrv != nil {
		s.broadcastSrv.stop()
	}
	s.grpcServer.Stop()
}

// ServerCtx is a context that is passed from the Gorums server to the handler.
// It allows the handler to release its lock on the server, allowing the next request to be processed.
// This happens automatically when the handler returns.
type ServerCtx struct {
	context.Context
	once *sync.Once // must be a pointer to avoid passing ctx by value
	mut  *sync.Mutex
}

// Release releases this handler's lock on the server, which allows the next request to be processed.
func (ctx *ServerCtx) Release() {
	ctx.once.Do(ctx.mut.Unlock)
}
