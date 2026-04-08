package gorums

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

// TestWithServerOptionsFiltersNil verifies that WithServerOptions silently drops
// nil ServerOptions rather than storing them, which would cause a panic when
// NewSystem or NewLocalSystems later calls NewServer with the collected options.
func TestWithServerOptionsFiltersNil(t *testing.T) {
	opts := newDialOptions()
	WithServerOptions(nil, WithReceiveBufferSize(8), nil)(&opts)
	if got := len(opts.srvOpts); got != 1 {
		t.Errorf("WithServerOptions: got %d srvOpts, want 1 (nil options must be dropped)", got)
	}
}

// TestWithMetadataJoinsInsteadOfOverwrites verifies that WithMetadata joins its
// argument with any previously set metadata rather than overwriting it. This is
// important when WithServer is applied before a user-supplied WithMetadata,
// because the node-id key set by WithServer must survive the subsequent
// WithMetadata call.
func TestWithMetadataJoinsInsteadOfOverwrites(t *testing.T) {
	const nodeIDKey = "x-gorums-node-id"

	opts := newDialOptions()

	// Simulate what WithServer does: set node-id metadata first.
	opts.metadata = metadata.Join(opts.metadata, metadata.Pairs(nodeIDKey, "42"))

	// Now apply a user-supplied WithMetadata; it must not clobber the node-id.
	WithMetadata(metadata.Pairs("x-custom", "hello"))(&opts)

	if vals := opts.metadata.Get(nodeIDKey); len(vals) == 0 {
		t.Errorf("WithMetadata overwrote %q metadata set by WithServer; got none", nodeIDKey)
	}
	if vals := opts.metadata.Get("x-custom"); len(vals) == 0 {
		t.Errorf("WithMetadata did not retain user-supplied key %q", "x-custom")
	}
}
