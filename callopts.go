package gorums

import "google.golang.org/protobuf/runtime/protoimpl"

// CallOption is a an option that can change the behavior of a gorums call.
type CallOption interface{}

type callOptions struct {
	callType      *protoimpl.ExtensionInfo
	channel       *Channel
	channels      []*Channel
	noSendWaiting bool
}

func (co callOptions) getChannel(n *Node) *Channel {
	if co.channel == nil {
		return n.channel
	}
	return co.channel
}

func (co callOptions) getChannels(cfg Configuration) []*Channel {
	if co.channels == nil {
		return cfg.Channels()
	}
	return co.channels
}

// callOptionFunc is a function that sets a value in the given callOptions struct
type callOptionFunc func(*callOptions)

func getCallOptions(callType *protoimpl.ExtensionInfo, opts []CallOption) callOptions {
	callOpts := callOptions{callType: callType}
	for _, o := range opts {
		switch opt := o.(type) {
		case callOptionFunc:
			opt(&callOpts)
		case *Channel:
			callOpts.channel = opt
		case []*Channel:
			callOpts.channels = opt
		}
	}
	return callOpts
}

// WithNoSendWaiting is a CallOption that makes Unicast or Multicast methods
// return immediately instead of blocking until the message has been sent.
func WithNoSendWaiting() callOptionFunc {
	return func(o *callOptions) {
		o.noSendWaiting = true
	}
}
