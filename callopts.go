package gorums

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type callOptions struct {
	callType     *protoimpl.ExtensionInfo
	waitSendDone bool
	streaming    bool  // streaming indicates whether this is a streaming call
	interceptors []any // Type-erased interceptors, restored by QuorumCallWithInterceptor
}

// mustWaitSendDone returns true if the caller of a one-way call type must wait
// for send completion. This is the default behavior unless the WithNoSendWaiting
// call option is set. This always returns false for two-way call types, since
// they should always wait for actual server responses.
func (o callOptions) mustWaitSendDone() bool {
	if o.callType == E_Rpc || o.callType == E_Quorumcall || o.callType == E_Correctable {
		return false
	}
	return o.callType != nil && o.waitSendDone
}

// CallOption is a function that sets a value in the given callOptions struct
type CallOption func(*callOptions)

func getCallOptions(callType *protoimpl.ExtensionInfo, opts ...CallOption) callOptions {
	o := callOptions{
		callType:     callType,
		waitSendDone: true, // default: wait for send completion
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithNoSendWaiting is a CallOption that makes Unicast or Multicast methods
// return immediately instead of blocking until the message has been sent.
// By default, Unicast and Multicast methods wait for send completion.
func WithNoSendWaiting() CallOption {
	return func(o *callOptions) {
		o.waitSendDone = false
	}
}

// Interceptors returns a CallOption that adds quorum call interceptors.
// Interceptors are executed in the order provided, modifying the Responses object
// before the user calls a terminal method.
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

// WithStreaming returns a CallOption that enables streaming mode for correctable calls.
// In streaming mode, the response iterator continues indefinitely until the context
// is canceled, allowing the server to send multiple responses over time.
// The reply channel buffer is also increased (10x) to handle streaming volume.
func WithStreaming() CallOption {
	return func(o *callOptions) {
		o.streaming = true
	}
}
