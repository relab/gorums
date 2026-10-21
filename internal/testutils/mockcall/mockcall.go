// Package mockcall provides typed wrappers around the [gorumsimpl] call
// entry points for tests that use the mock service. The wrappers fix the
// request and response types to [wrapperspb.StringValue] (or
// [wrapperspb.Int32Value] for the Int32 variants), so call sites need not
// spell out type arguments.
//
// Tests that exercise the gorumsimpl entry points themselves should call
// gorumsimpl directly.
package mockcall

import (
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// QuorumCall returns a quorum call of method with msg as the request.
func QuorumCall(ctx *gorums.ConfigContext, method, msg string) *gorums.Call[*wrapperspb.StringValue, *wrapperspb.StringValue] {
	return gorumsimpl.QuorumCall[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), method)
}

// QuorumCallStream returns a streaming quorum call of method with msg as the request.
func QuorumCallStream(ctx *gorums.ConfigContext, method, msg string) *gorums.Call[*wrapperspb.StringValue, *wrapperspb.StringValue] {
	return gorumsimpl.QuorumCallStream[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), method)
}

// Int32QuorumCall returns a quorum call of method with val as the request.
func Int32QuorumCall(ctx *gorums.ConfigContext, method string, val int32) *gorums.Call[*wrapperspb.Int32Value, *wrapperspb.Int32Value] {
	return gorumsimpl.QuorumCall[*wrapperspb.Int32Value, *wrapperspb.Int32Value](ctx, wrapperspb.Int32(val), method)
}

// RemoteCall sends msg to a single node and returns its response.
func RemoteCall(ctx *gorums.NodeContext, method, msg string) (*wrapperspb.StringValue, error) {
	return gorumsimpl.RemoteCall[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), method)
}

// Multicast returns a one-way call of method to every node in the configuration.
func Multicast(ctx *gorums.ConfigContext, method, msg string) *gorums.OnewayCall[*wrapperspb.StringValue] {
	return gorumsimpl.Multicast(ctx, wrapperspb.String(msg), method)
}

// Unicast returns a one-way call of method to a single node.
func Unicast(ctx *gorums.NodeContext, method, msg string) *gorums.OnewayCall[*wrapperspb.StringValue] {
	return gorumsimpl.Unicast(ctx, wrapperspb.String(msg), method)
}

// MustMulticast sends msg to every node and fails the test if Send returns an error.
func MustMulticast(t testing.TB, ctx *gorums.ConfigContext, method, msg string) {
	t.Helper()
	if err := Multicast(ctx, method, msg).Send(); err != nil {
		t.Fatalf("Multicast(%q) failed: %v", method, err)
	}
}

// MustRemoteCall calls method on a single node and fails the test on error.
func MustRemoteCall(t testing.TB, ctx *gorums.NodeContext, method, msg string) *wrapperspb.StringValue {
	t.Helper()
	resp, err := RemoteCall(ctx, method, msg)
	if err != nil {
		t.Fatalf("RemoteCall(%q) failed: %v", method, err)
	}
	return resp
}
