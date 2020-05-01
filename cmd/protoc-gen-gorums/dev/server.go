package dev

import (
	"net"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
)

// requestHandler is used to fetch a response message based on the request.
// A requestHandler should receive a message from the server, unmarshal it into
// the proper type for that Method's request type, call a user provided Handler,
// and return a marshaled result to the server.
type requestHandler func(*ordering.Message) *ordering.Message

type strictOrderingServer struct {
	handlers map[string]requestHandler
}

func newStrictOrderingServer() *strictOrderingServer {
	return &strictOrderingServer{
		handlers: make(map[string]requestHandler),
	}
}

func (s *strictOrderingServer) registerHandler(method string, handler requestHandler) {
	s.handlers[method] = handler
}

func (s *strictOrderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	for {
		req, err := srv.Recv()
		if err != nil {
			return err
		}
		// handle the request if a handler is available for this rpc
		if handler, ok := s.handlers[req.GetMethod()]; ok {
			resp := handler(req)
			resp.ID = req.GetID()
			err = srv.Send(resp)
			if err != nil {
				return err
			}
		}
	}
}

// GorumsServer serves all strict ordering based RPCs using registered handlers
type GorumsServer struct {
	srv        *strictOrderingServer
	grpcServer *grpc.Server
}

// NewGorumsServer returns a new instance of GorumsServer
func NewGorumsServer() *GorumsServer {
	s := &GorumsServer{
		srv:        newStrictOrderingServer(),
		grpcServer: grpc.NewServer(),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// Serve starts serving on the listener
func (s *GorumsServer) Serve(listener net.Listener) {
	s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *GorumsServer) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately
func (s *GorumsServer) Stop() {
	s.grpcServer.Stop()
}
