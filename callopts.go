package gorums

import (
	"google.golang.org/protobuf/runtime/protoimpl"
)

type callOptions struct {
	callType     *protoimpl.ExtensionInfo
	ignoreErrors bool
	interceptors []any // Type-erased interceptors, restored by QuorumCall
}

// mustWaitSendDone returns true if the caller of a one-way call type must wait
// for send completion. This is the default behavior unless the IgnoreErrors
// call option is set. This always returns false for two-way call types, since
// they should always wait for actual server responses.
func (o callOptions) mustWaitSendDone() bool {
	// must wait for send completion if we are not ignoring errors
	// and the call type is Unicast or Multicast
	return !o.ignoreErrors && (o.callType == E_Unicast || o.callType == E_Multicast)
}

// CallOption is a function that sets a value in the given callOptions struct
type CallOption func(*callOptions)

func getCallOptions(callType *protoimpl.ExtensionInfo, opts ...CallOption) callOptions {
	o := callOptions{
		callType:     callType,
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
// Interceptors are executed in the order provided, modifying the Responses object
// before the user calls a terminal method.
//
// Example:
//
//	resp, err := ReadQC(ctx, req,
//	    gorums.Interceptors(loggingInterceptor, filterInterceptor),
//	).Majority()
func Interceptors[T NodeID, Req, Resp msg](interceptors ...QuorumInterceptor[T, Req, Resp]) CallOption {
	return func(o *callOptions) {
		for _, interceptor := range interceptors {
			o.interceptors = append(o.interceptors, interceptor)
		}
	}
}
