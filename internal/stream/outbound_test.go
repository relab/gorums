package stream

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestOutboundRecvBufferSize verifies that an outbound channel sizes its
// request dispatch queue from OutboundOptions.RecvBufferSize.
func TestOutboundRecvBufferSize(t *testing.T) {
	for _, size := range []uint{1, 7} {
		conn, err := grpc.NewClient("127.0.0.1:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("NewClient() error: %v", err)
		}
		c := NewOutboundChannel(t.Context(), 1, conn, OutboundOptions{SendBufferSize: 1, RecvBufferSize: size})
		t.Cleanup(func() { _ = c.Close() })
		if got := cap(c.requests.slots); got != int(size) {
			t.Errorf("dispatch capacity = %d, want %d", got, size)
		}
	}
}
