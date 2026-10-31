package stream

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestOutboundDispatchSize verifies that an outbound channel sizes its request
// dispatch queue from OutboundOptions.DispatchSize, and that 0 selects the
// default.
func TestOutboundDispatchSize(t *testing.T) {
	tests := []struct {
		name string
		size uint
		want int
	}{
		{name: "Zero", size: 0, want: defaultRequestDispatchSize},
		{name: "Explicit", size: 7, want: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := grpc.NewClient("127.0.0.1:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("NewClient() error: %v", err)
			}
			c := NewOutboundChannel(t.Context(), 1, conn, OutboundOptions{SendBufferSize: 1, DispatchSize: tt.size})
			t.Cleanup(func() { _ = c.Close() })
			if got := cap(c.requests.slots); got != tt.want {
				t.Errorf("dispatch capacity = %d, want %d", got, tt.want)
			}
		})
	}
}
