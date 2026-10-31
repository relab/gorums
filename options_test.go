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
	srv := NewServer(nil, WithBufferSizes(8, 8), nil) //nolint:gocritic // nil in several positions is the case under test
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	srv.Stop()
}

// TestWithDialBufferSizes verifies that WithDialBufferSizes sets both buffer
// sizes of outbound channels, and that 0 selects the default for each: the
// receive size resolves to the default in the stream layer and stays 0 here,
// while the send size resolves to [conn.DefaultSendBufferSize].
func TestWithDialBufferSizes(t *testing.T) {
	tests := []struct {
		name     string
		opt      DialOption
		wantRecv uint
		wantSend uint
	}{
		{name: "Unset", opt: nil, wantRecv: 0, wantSend: conn.DefaultSendBufferSize},
		{name: "Zero", opt: WithDialBufferSizes(0, 0), wantRecv: 0, wantSend: conn.DefaultSendBufferSize},
		{name: "Explicit", opt: WithDialBufferSizes(128, 64), wantRecv: 128, wantSend: 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := conn.NewDialOptions()
			if tt.opt != nil {
				tt.opt(&opts)
			}
			if opts.RecvBufferSize != tt.wantRecv {
				t.Errorf("RecvBufferSize = %d, want %d", opts.RecvBufferSize, tt.wantRecv)
			}
			if opts.SendBufferSize != tt.wantSend {
				t.Errorf("SendBufferSize = %d, want %d", opts.SendBufferSize, tt.wantSend)
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
