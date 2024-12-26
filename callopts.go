package gorums

import "google.golang.org/protobuf/runtime/protoimpl"

type callOptions struct {
	callType      *protoimpl.ExtensionInfo
	noSendWaiting bool
	signOrigin    bool
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

// WithNoSendWaiting is a CallOption that makes Unicast or Multicast methods
// return immediately instead of blocking until the message has been sent.
func WithNoSendWaiting() CallOption {
	return func(o *callOptions) {
		o.noSendWaiting = true
	}
}

// WithOriginAuthentication is a CallOption that makes BroadcastCall methods
// digitally sign messages.
func WithOriginAuthentication() CallOption {
	return func(o *callOptions) {
		o.signOrigin = true
	}
}
