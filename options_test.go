package gorums

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

// TestNewServerToleratesNilOptions verifies that NewServer skips nil
// ServerOptions rather than panicking, so callers that thread an optional
// option (for example [NewLocalServers]) can pass nil.
func TestNewServerToleratesNilOptions(t *testing.T) {
	srv := NewServer(nil, WithBufferSizes(8, 8), nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	srv.Stop()
}

// TestWithMetadataJoinsInsteadOfOverwrites verifies that WithMetadata joins its
// argument with any previously set metadata rather than overwriting it. This is
// important when WithBackChannel is applied before a user-supplied WithMetadata,
// because the node-id key set by WithBackChannel must survive the subsequent
// WithMetadata call.
func TestWithMetadataJoinsInsteadOfOverwrites(t *testing.T) {
	const nodeIDKey = "x-gorums-node-id"

	opts := newDialOptions()

	// Simulate what WithBackChannel does: set node-id metadata first.
	opts.metadata = metadata.Join(opts.metadata, metadata.Pairs(nodeIDKey, "42"))

	// Now apply a user-supplied WithMetadata; it must not clobber the node-id.
	WithMetadata(metadata.Pairs("x-custom", "hello"))(&opts)

	if vals := opts.metadata.Get(nodeIDKey); len(vals) == 0 {
		t.Errorf("WithMetadata overwrote %q metadata set by WithBackChannel; got none", nodeIDKey)
	}
	if vals := opts.metadata.Get("x-custom"); len(vals) == 0 {
		t.Errorf("WithMetadata did not retain user-supplied key %q", "x-custom")
	}
}
