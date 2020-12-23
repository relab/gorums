package gorums

import "google.golang.org/protobuf/runtime/protoimpl"

type callOptions struct {
	callType  *protoimpl.ExtensionInfo
	sendAsync bool
}

// CallOption is a function that sets a value in the given callOptions struct
type CallOption func(*callOptions)

func getCallOptions(callType *protoimpl.ExtensionInfo, opts []CallOption) callOptions {
	o := callOptions{callType: callType}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithAsyncSend is a CallOption that makes Unicast or Multicast methods run asynchronously,
// instead of blocking until the message is sent.
func WithAsyncSend() CallOption {
	return func(o *callOptions) {
		o.sendAsync = true
	}
}
