package interceptors

import (
	"fmt"
	"log"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/peer"
)

func LoggingInterceptor(addr string) func(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	return func(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		log.Printf("[%s]: LoggingInterceptor(incoming): Method=%s, Message=%s", addr, msg.Metadata.GetMethod(), msg.Message)
		start := time.Now()
		resp, err := next(ctx, msg)

		duration := time.Since(start)
		log.Printf("[%s]: LoggingInterceptor(outgoing): Method=%s, Duration=%s, ResponseErr=%v, Message=%v, Type=%T", addr, msg.Metadata.GetMethod(), duration, resp.Metadata, resp.Message, resp.Message)
		return resp, err
	}
}

func LoggingSimpleInterceptor(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	log.Printf("LoggingSimpleInterceptor(incoming): Method=%s, Message=%v)", msg.Metadata.GetMethod(), msg.Message)
	resp, err := next(ctx, msg)
	log.Printf("LoggingSimpleInterceptor(outgoing): Method=%s, ResponseErr=%v, Message=%v", msg.Metadata.GetMethod(), err, resp.Message)
	return resp, err
}

func DelayedInterceptor(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	// delay based on sending node address
	delay := 0 * time.Millisecond
	peer, ok := peer.FromContext(ctx)
	if ok && peer.Addr != nil {
		node := peer.Addr.String()
		log.Printf("DelayedInterceptor: Received message from node %v", peer)
		// Example: delay based on node address length
		delay = time.Duration(len(node)) * 100 * time.Millisecond
		log.Printf("DelayedInterceptor: Delaying message processing for %s based on node address length", delay)
	} else {
		log.Printf("DelayedInterceptor: No peer address found in context, using default delay of 0ms")
	}

	time.Sleep(delay)
	// Call the next handler in the chain
	resp, err := next(ctx, msg)
	log.Printf("DelayedInterceptor: Finished processing message after %s", delay)
	return resp, err
}

/** NoFooAllowedInterceptor rejects requests for messages with key "foo". */
func NoFooAllowedInterceptor[T interface{ GetKey() string }](ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	if req, ok := msg.Message.(T); ok {
		log.Printf("NoFooAllowedInterceptor: Received request for key '%s'", req.GetKey())
		if req.GetKey() == "foo" {
			log.Printf("NoFooAllowedInterceptor: Rejecting request for key 'foo'")
			return nil, fmt.Errorf("requests for key 'foo' are not allowed")
		}
	}
	return next(ctx, msg)
}

func MetadataInterceptor(ctx gorums.ServerCtx, msg *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
	log.Printf("MetadataInterceptor: Adding custom metadata to message(customKey=customValue)")
	// Add a custom metadata field
	entry := ordering.MetadataEntry_builder{
		Key:   "customKey",
		Value: "customValue",
	}.Build()
	msg.Metadata.SetEntry([]*ordering.MetadataEntry{
		entry,
	})
	// Call the next handler in the chain
	resp, err := next(ctx, msg)
	log.Printf("MetadataInterceptor: Finished processing message with custom metadata")
	return resp, err
}
