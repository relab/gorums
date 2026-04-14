package gorums

import (
	"google.golang.org/protobuf/proto"
)

type callOptions struct {
	ignoreErrors bool
	interceptors []any // Type-erased interceptors, restored by QuorumCall
}

// CallOption is a function that sets a value in the given callOptions struct
type CallOption func(*callOptions)

func getCallOptions(opts ...CallOption) callOptions {
	o := callOptions{
		ignoreErrors: false, // default: return error and wait for send completion
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// IgnoreErrors ignores send errors from Unicast or Multicast methods and
// returns immediately instead of blocking until the message has been sent.
// By default, Unicast and Multicast methods return an error if the message
// could not be sent or the context was canceled.
func IgnoreErrors() CallOption {
	return func(o *callOptions) {
		o.ignoreErrors = true
	}
}

// Interceptors returns a CallOption that adds quorum call interceptors.
// Interceptors are executed in the order provided, modifying the Responses
// object before the user calls a terminal method.
//
// Example:
//
//	resp, err := ReadQC(ctx, req,
//	    gorums.Interceptors(loggingInterceptor, filterInterceptor),
//	).Majority()
func Interceptors[Req, Resp proto.Message](interceptors ...QuorumInterceptor[Req, Resp]) CallOption {
	return func(o *callOptions) {
		for _, interceptor := range interceptors {
			o.interceptors = append(o.interceptors, interceptor)
		}
	}
}
