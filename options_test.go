package gorums

import (
	"testing"

	"github.com/relab/gorums/internal/conn"
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

// TestWithSendBufferSizeDefault verifies that the send queue capacity defaults
// to defaultSendBufferSize and that an explicit size of 0 selects the same
// default. Capacity 0 is not viable under the full-queue fail-fast semantics:
// every two-way request enqueued while the sender is busy would fail.
func TestWithSendBufferSizeDefault(t *testing.T) {
	tests := []struct {
		name string
		opt  DialOption
		want uint
	}{
		{name: "Unset", opt: nil, want: conn.DefaultSendBufferSize},
		{name: "Zero", opt: WithSendBufferSize(0), want: conn.DefaultSendBufferSize},
		{name: "Explicit", opt: WithSendBufferSize(128), want: 128},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := conn.NewDialOptions()
			if tt.opt != nil {
				tt.opt(&opts)
			}
			if opts.SendBuffer != tt.want {
				t.Errorf("sendBuffer = %d, want %d", opts.SendBuffer, tt.want)
			}
		})
	}
}

// TestWithMetadataJoinsInsteadOfOverwrites verifies that WithMetadata joins its
// argument with any previously set metadata rather than overwriting it. This is
// important when withServer is applied before a user-supplied WithMetadata,
// because the node-id key set by withServer must survive the subsequent
// WithMetadata call.
func TestWithMetadataJoinsInsteadOfOverwrites(t *testing.T) {
	const nodeIDKey = "x-gorums-node-id"

	opts := conn.NewDialOptions()

	// Simulate what withServer does: set node-id metadata first.
	opts.Metadata = metadata.Join(opts.Metadata, metadata.Pairs(nodeIDKey, "42"))

	// Now apply a user-supplied WithMetadata; it must not clobber the node-id.
	WithMetadata(metadata.Pairs("x-custom", "hello"))(&opts)

	if vals := opts.Metadata.Get(nodeIDKey); len(vals) == 0 {
		t.Errorf("WithMetadata overwrote %q metadata set by withServer; got none", nodeIDKey)
	}
	if vals := opts.Metadata.Get("x-custom"); len(vals) == 0 {
		t.Errorf("WithMetadata did not retain user-supplied key %q", "x-custom")
	}
}
