package gorums

import (
	"context"
	"net"
	"sync"

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
	md.SetStatus(errStatus.Proto())
	return &Message{Metadata: md, Message: resp}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *Message, s.opts.buffer)
	ctx := srv.Context()

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
		if handler, ok := s.handlers[req.Metadata.GetMethod()]; ok {
			// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
			// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
			// finished, or when it is safe to start processing the next request.
			go handler(ServerCtx{Context: req.Metadata.AppendToIncomingContext(ctx), once: new(sync.Once), mut: &mut}, req, finished)
			// Wait until the handler releases the mutex.
			mut.Lock()
		}
	}
}

type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
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

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv        *orderingServer
	grpcServer *grpc.Server
}

// NewServer returns a new instance of [gorums.Server].
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	s := &Server{
		srv:        newOrderingServer(&serverOpts),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler requestHandler) {
	s.srv.handlers[method] = handler
}

// Serve starts serving on the listener.
func (s *Server) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately.
func (s *Server) Stop() {
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
