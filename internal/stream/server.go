package stream

import (
	"context"
	"sync"
)

// MessageHandler handles an incoming stream message on the server side.
// It is called in a new goroutine for each received message.
// The handler must eventually release the mutex (by calling mut.Unlock())
// to allow the next request to be processed. The finished channel is used
// to send response messages back to the client.
type MessageHandler func(ctx context.Context, mut *sync.Mutex, finished chan<- *Message, msg *Message)

// Server implements the Gorums gRPC service for handling node streams.
type Server struct {
	handlers        map[string]MessageHandler
	buffer          uint
	connectCallback func(context.Context)
	UnimplementedGorumsServer
}

// NewServer creates a new StreamServer with the given buffer size
// and optional connect callback.
func NewServer(buffer uint, connectCallback func(context.Context)) *Server {
	return &Server{
		handlers:        make(map[string]MessageHandler),
		buffer:          buffer,
		connectCallback: connectCallback,
	}
}

// RegisterHandler registers a message handler for the specified method name.
func (s *Server) RegisterHandler(method string, handler MessageHandler) {
	s.handlers[method] = handler
}

// NodeStream handles a connection to a single client. The stream is aborted if there
// is any error with sending or receiving.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	var mut sync.Mutex
	finished := make(chan *Message, s.buffer)
	ctx := srv.Context()

	if s.connectCallback != nil {
		s.connectCallback(ctx)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case streamOut := <-finished:
				if err := srv.Send(streamOut); err != nil {
					return
				}
			}
		}
	}()

	// Start with a locked mutex
	mut.Lock()
	defer mut.Unlock()

	for {
		streamIn, err := srv.Recv()
		if err != nil {
			return err
		}
		if handler, ok := s.handlers[streamIn.GetMethod()]; ok {
			// We start the handler in a new goroutine in order to allow multiple
			// handlers to run concurrently. However, to preserve request ordering,
			// the handler must unlock the shared mutex when it has either finished,
			// or when it is safe to start processing the next request.
			go handler(ctx, &mut, finished, streamIn)
			// Wait until the handler releases the mutex.
			mut.Lock()
		}
	}
}
