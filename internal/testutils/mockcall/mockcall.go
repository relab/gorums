// Package mockcall provides typed wrappers around the [gorumsimpl] call
// entry points for tests that use the mock service. The wrappers fix the
// request and response types to [wrapperspb.StringValue] (or
// [wrapperspb.Int32Value] for [Int32QuorumCall]) and fix the method to
// [mock.TestMethod], so call sites need not spell out type arguments or a
// method name.
//
// Tests that need another method, or that exercise the gorumsimpl entry
// points themselves, should call gorumsimpl directly.
package mockcall

import (
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// QuorumCall returns a quorum call of [mock.TestMethod] with msg as the request.
func QuorumCall(ctx *gorums.ConfigContext, msg string) *gorums.Call[*wrapperspb.StringValue, *wrapperspb.StringValue] {
	return gorumsimpl.QuorumCall[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), mock.TestMethod)
}

// QuorumCallStream returns a streaming quorum call of [mock.StreamMethod]
// with msg as the request.
func QuorumCallStream(ctx *gorums.ConfigContext, msg string) *gorums.Call[*wrapperspb.StringValue, *wrapperspb.StringValue] {
	return gorumsimpl.QuorumCallStream[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), mock.StreamMethod)
}

// Int32QuorumCall returns a quorum call of [mock.GetValueMethod] with val as
// the request.
func Int32QuorumCall(ctx *gorums.ConfigContext, val int32) *gorums.Call[*wrapperspb.Int32Value, *wrapperspb.Int32Value] {
	return gorumsimpl.QuorumCall[*wrapperspb.Int32Value, *wrapperspb.Int32Value](ctx, wrapperspb.Int32(val), mock.GetValueMethod)
}

// RemoteCall sends msg to a single node as [mock.TestMethod] and returns its response.
func RemoteCall(ctx *gorums.NodeContext, msg string) (*wrapperspb.StringValue, error) {
	return gorumsimpl.RemoteCall[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), mock.TestMethod)
}

// EchoRemoteCall sends msg to a single node as [mock.EchoMethod] and returns its response.
func EchoRemoteCall(ctx *gorums.NodeContext, msg string) (*wrapperspb.StringValue, error) {
	return gorumsimpl.RemoteCall[*wrapperspb.StringValue, *wrapperspb.StringValue](ctx, wrapperspb.String(msg), mock.EchoMethod)
}

// Multicast returns a one-way call of [mock.TestMethod] to every node in the
// configuration.
func Multicast(ctx *gorums.ConfigContext, msg string) *gorums.OnewayCall[*wrapperspb.StringValue] {
	return gorumsimpl.Multicast(ctx, wrapperspb.String(msg), mock.TestMethod)
}

// Unicast returns a one-way call of [mock.TestMethod] to a single node.
func Unicast(ctx *gorums.NodeContext, msg string) *gorums.OnewayCall[*wrapperspb.StringValue] {
	return gorumsimpl.Unicast(ctx, wrapperspb.String(msg), mock.TestMethod)
}

// MustMulticast sends msg to every node and fails the test if Send returns an error.
func MustMulticast(t testing.TB, ctx *gorums.ConfigContext, msg string) {
	t.Helper()
	if err := Multicast(ctx, msg).Send(); err != nil {
		t.Fatalf("Multicast failed: %v", err)
	}
}

// MustRemoteCall calls [mock.TestMethod] on a single node and fails the test on error.
func MustRemoteCall(t testing.TB, ctx *gorums.NodeContext, msg string) *wrapperspb.StringValue {
	t.Helper()
	resp, err := RemoteCall(ctx, msg)
	if err != nil {
		t.Fatalf("RemoteCall failed: %v", err)
	}
	return resp
}
