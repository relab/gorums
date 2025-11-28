package gorums

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type callOptions struct {
	callType     *protoimpl.ExtensionInfo
	waitSendDone bool
	transform    func(proto.Message, *RawNode) proto.Message
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

func interceptorsFromCallOptions[Req, Resp proto.Message, Out any](o callOptions) []QuorumInterceptor[Req, Resp, Out] {
	interceptors := make([]QuorumInterceptor[Req, Resp, Out], len(o.interceptors))
	for i, ic := range o.interceptors {
		interceptors[i] = ic.(QuorumInterceptor[Req, Resp, Out])
	}
	return interceptors
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
// Multiple interceptors are executed in the order provided, wrapping the base
// quorum function.
//
// Example:
//
//	resp, err := cfg.Read(ctx, req,
//	    gorums.Interceptors(loggingInterceptor, retryInterceptor),
//	)
func Interceptors[Req, Resp proto.Message, Out any](interceptors ...QuorumInterceptor[Req, Resp, Out]) CallOption {
	return func(o *callOptions) {
		for _, interceptor := range interceptors {
			o.interceptors = append(o.interceptors, interceptor)
		}
	}
}

// Transform returns a CallOption that applies per-node request transformations
// for Unicast or Multicast calls. The transform function receives the original request
// and a node, and returns the transformed request to send to that node.
// If the function returns nil or an invalid message, the request to that node is skipped.
//
// Example:
//
//	config.Multicast(ctx, req, gorums.Transform(
//	    func(req *Request, node *gorums.RawNode) *Request {
//	        return &Request{Value: fmt.Sprintf("%s-%d", req.Value, node.ID())}
//	    },
//	))
func Transform[Req proto.Message](fn func(Req, *RawNode) Req) CallOption {
	return func(o *callOptions) {
		if o.transform == nil {
			// First transform
			o.transform = func(req proto.Message, node *RawNode) proto.Message {
				return fn(req.(Req), node)
			}
		} else {
			// Chain with existing transform
			prev := o.transform
			o.transform = func(req proto.Message, node *RawNode) proto.Message {
				intermediate := prev(req, node)
				if intermediate == nil {
					return nil
				}
				return fn(intermediate.(Req), node)
			}
		}
	}
}
