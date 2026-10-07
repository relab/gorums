// Package interceptors shows how to write Gorums server interceptors, such as a
// logging interceptor that reports each incoming request and its response.
package interceptors

import (
	"errors"
	"log"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
)

// LoggingInterceptor returns an interceptor that logs each request and its
// response, tagged with addr, together with the time the handler took.
func LoggingInterceptor(addr string) gorums.ServerInterceptor {
	return func(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		req := gorums.AsProto[proto.Message](in)
		log.Printf("[%s]: LoggingInterceptor(incoming): Method=%s, Message=%s", addr, in.GetMethod(), req)
		start := time.Now()
		out, err := next(ctx, in)

		duration := time.Since(start)
		resp := gorums.AsProto[proto.Message](out)
		log.Printf("[%s]: LoggingInterceptor(outgoing): Method=%s, Duration=%s, Err=%v, Message=%v, Type=%T", addr, in.GetMethod(), duration, err, resp, resp)
		return out, err
	}
}

// LoggingSimpleInterceptor logs each request and its response.
func LoggingSimpleInterceptor(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	req := gorums.AsProto[proto.Message](in)
	log.Printf("LoggingSimpleInterceptor(incoming): Method=%s, Message=%v)", in.GetMethod(), req)
	out, err := next(ctx, in)
	resp := gorums.AsProto[proto.Message](out)
	log.Printf("LoggingSimpleInterceptor(outgoing): Method=%s, Err=%v, Message=%v", in.GetMethod(), err, resp)
	return out, err
}

// DelayedInterceptor delays each request before passing it on. The delay grows
// with the length of the sender's network address; without a known sender
// address there is no delay.
func DelayedInterceptor(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	// delay based on sending node address
	delay := 0 * time.Millisecond
	p, ok := peer.FromContext(ctx)
	if ok && p.Addr != nil {
		node := p.Addr.String()
		log.Printf("DelayedInterceptor: Received message from node %v", p)
		// Example: delay based on node address length
		delay = time.Duration(len(node)) * 100 * time.Millisecond
		log.Printf("DelayedInterceptor: Delaying message processing for %s based on node address length", delay)
	} else {
		log.Printf("DelayedInterceptor: No peer address found in context, using default delay of 0ms")
	}

	time.Sleep(delay)
	// Call the next handler in the chain
	out, err := next(ctx, in)
	log.Printf("DelayedInterceptor: Finished processing message after %s", delay)
	return out, err
}

// NoFooAllowedInterceptor rejects requests for messages with key "foo".
func NoFooAllowedInterceptor[T interface{ GetKey() string }](ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	if req, ok := gorums.AsProto[proto.Message](in).(T); ok {
		log.Printf("NoFooAllowedInterceptor: Received request for key '%s'", req.GetKey())
		if req.GetKey() == "foo" {
			log.Printf("NoFooAllowedInterceptor: Rejecting request for key 'foo'")
			return nil, errors.New("requests for key 'foo' are not allowed")
		}
	}
	return next(ctx, in)
}

// MetadataInterceptor sets each request's metadata to the single entry
// customKey=customValue before passing the request on.
func MetadataInterceptor(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	log.Printf("MetadataInterceptor: Adding custom metadata to message(customKey=customValue)")
	// Add a custom metadata field
	entry := &gorums.MetadataEntry{}
	entry.SetKey("customKey")
	entry.SetValue("customValue")
	in.SetEntry([]*gorums.MetadataEntry{entry})

	// Call the next handler in the chain
	out, err := next(ctx, in)
	log.Printf("MetadataInterceptor: Finished processing message with custom metadata")
	return out, err
}
