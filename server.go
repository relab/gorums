package gorums

import (
	"context"
	"net"
	"sync"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
)

type (
	// Interceptor is a function that can intercept and modify incoming requests
	// and outgoing responses. It receives a ServerCtx, the incoming Message, and
	// a Handler representing the next element in the chain (either another
	// Interceptor or the actual server method). It returns a Message and an error.
	Interceptor func(ServerCtx, *Message, Handler) (*Message, error)
	// Handler is a function that processes a request message and returns a response message.
	Handler func(ServerCtx, *Message) (*Message, error)
)

type orderingServer struct {
	handlers map[string]Handler
	opts     *serverOptions
	ordering.UnimplementedGorumsServer
}

func newOrderingServer(opts *serverOptions) *orderingServer {
	return &orderingServer{
		handlers: make(map[string]Handler),
		opts:     opts,
	}
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *ordering.Metadata, s.opts.buffer)
	ctx := srv.Context()

	if s.opts.connectCallback != nil {
		s.opts.connectCallback(ctx)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case md := <-finished:
				if err := srv.Send(md); err != nil {
					return
				}
			}
		}
	}()

	// Start with a locked mutex
	mut.Lock()
	defer mut.Unlock()

	for {
		md, err := srv.Recv()
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[md.GetMethod()]; ok {
			// We start the handler in a new goroutine in order to allow multiple handlers to run concurrently.
			// However, to preserve request ordering, the handler must unlock the shared mutex when it has either
			// finished, or when it is safe to start processing the next request.
			//
			// This func() is the default interceptor; it is the first and last handler in the chain.
			// It is responsible for releasing the mutex when the handler chain is done.
			go func() {
				srvCtx := newServerCtx(md.AppendToIncomingContext(ctx), &mut, finished)
				defer srvCtx.Release()

				req, err := UnmarshalRequest(md)
				if err != nil {
					_ = srvCtx.SendMessage(responseWithError(nil, md, err))
					return
				}

				message, err := handler(srvCtx, req)
				// If there is no message and no error, we do not send anything back to the client.
				// This corresponds to a unidirectional message from client to server, where clients
				// are not expected to receive a response.
				if message == nil && err == nil {
					return
				}
				_ = srvCtx.SendMessage(responseWithError(message, md, err))
				// We ignore the error from SendMessage here; it means that the stream is closed.
				// The for-loop above will exit on the next Recv call.
			}()
			// Wait until the handler releases the mutex.
			mut.Lock()
		}
	}
}

type serverOptions struct {
	buffer          uint
	grpcOpts        []grpc.ServerOption
	connectCallback func(context.Context)
	interceptors    []Interceptor
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

// WithInterceptors registers server-side interceptors to run for every incoming request.
// Interceptors are executed for each registered handler. Interceptors may modify both
// the request and/or response messages, or perform additional actions before or after
// calling the next handler in the chain. Interceptors are executed in the order they are
// provided: the first element is executed first, and the last element calls the actual
// server method handler.
func WithInterceptors(i ...Interceptor) ServerOption {
	return func(opts *serverOptions) {
		opts.interceptors = append(opts.interceptors, i...)
	}
}

// chainInterceptors composes the provided interceptors around the final Handler and
// returns a Handler that executes the chain. The execution order is the same as the
// order of the interceptors in the slice: the first element is executed first, and
// the last element calls the final handler (the server method).
func chainInterceptors(final Handler, interceptors ...Interceptor) Handler {
	if len(interceptors) == 0 {
		return final
	}
	handler := final
	for i := len(interceptors) - 1; i >= 0; i-- {
		curr := interceptors[i]
		next := handler
		handler = func(ctx ServerCtx, msg *Message) (*Message, error) {
			return curr(ctx, msg, next)
		}
	}
	return handler
}

// Server serves all ordering based RPCs using registered handlers.
type Server struct {
	srv          *orderingServer
	grpcServer   *grpc.Server
	interceptors []Interceptor
}

// NewServer returns a new instance of [gorums.Server].
func NewServer(opts ...ServerOption) *Server {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	s := &Server{
		srv:          newOrderingServer(&serverOpts),
		grpcServer:   grpc.NewServer(serverOpts.grpcOpts...),
		interceptors: serverOpts.interceptors,
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// RegisterHandler registers a request handler for the specified method name.
//
// This function should only be used by generated code.
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.srv.handlers[method] = chainInterceptors(handler, s.interceptors...)
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
	c    chan<- *ordering.Metadata
}

// newServerCtx creates a new ServerCtx with the given context, mutex and metadata channel.
func newServerCtx(ctx context.Context, mut *sync.Mutex, c chan<- *ordering.Metadata) ServerCtx {
	return ServerCtx{
		Context: ctx,
		once:    new(sync.Once),
		mut:     mut,
		c:       c,
	}
}

// Release releases this handler's lock on the server, which allows the next request
// to be processed concurrently. Use Release only when the handler no longer needs
// exclusive access to the server's state. It is safe to call Release multiple times.
func (ctx *ServerCtx) Release() {
	ctx.once.Do(ctx.mut.Unlock)
}

// SendMessage attempts to send the given message to the client.
// This may fail if the stream was closed or the stream context got canceled.
//
// This function should be used by generated code only.
func (ctx *ServerCtx) SendMessage(msg *Message) error {
	md, err := MarshalResponseMetadata(msg)
	if err != nil {
		return err
	}
	select {
	case ctx.c <- md:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
