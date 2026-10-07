package gorums

import (
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestPublicTransportErrorsInspectable verifies that the exported transport
// sentinels carry the documented gRPC Unavailable code, are distinct under
// errors.Is, and remain matchable after a node error is aggregated into a
// QuorumCallError — the supported public inspection contract for callers that
// need to distinguish stream-down, closed-node, and queue-full failures.
func TestPublicTransportErrorsInspectable(t *testing.T) {
	for _, e := range []error{ErrStreamDown, ErrNodeClosed, ErrSendQueueFull} {
		if status.Code(e) != codes.Unavailable {
			t.Errorf("%v code = %v, want %v", e, status.Code(e), codes.Unavailable)
		}
	}
	if errors.Is(ErrStreamDown, ErrNodeClosed) ||
		errors.Is(ErrStreamDown, ErrSendQueueFull) ||
		errors.Is(ErrNodeClosed, ErrSendQueueFull) {
		t.Error("exported transport sentinels are not distinct under errors.Is")
	}

	qce := conn.NewQuorumCallError(ErrIncomplete, []conn.NodeError{conn.NewNodeError(1, ErrStreamDown)})
	if !errors.Is(qce, ErrStreamDown) {
		t.Error("errors.Is(QuorumCallError{ErrStreamDown}, ErrStreamDown) = false, want true")
	}
	if errors.Is(qce, ErrNodeClosed) {
		t.Error("errors.Is(QuorumCallError{ErrStreamDown}, ErrNodeClosed) = true, want false")
	}
}
