package gorums

import (
	"context"
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestQuorumCallErrorIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "SameCauseError",
			err:    conn.NewQuorumCallError(ErrIncomplete, nil),
			target: ErrIncomplete,
			want:   true,
		},
		{
			name:   "SameCauseQCError",
			err:    conn.NewQuorumCallError(ErrIncomplete, nil),
			target: conn.NewQuorumCallError(ErrIncomplete, nil),
			want:   true,
		},
		{
			name:   "DifferentError",
			err:    conn.NewQuorumCallError(ErrIncomplete, nil),
			target: errors.New("incomplete call"),
			want:   false,
		},
		{
			name:   "DifferentQCError",
			err:    conn.NewQuorumCallError(ErrIncomplete, nil),
			target: conn.NewQuorumCallError(errors.New("incomplete call"), nil),
			want:   false,
		},
		{
			name:   "ContextCanceled",
			err:    conn.NewQuorumCallError(context.Canceled, nil),
			target: context.Canceled,
			want:   true,
		},
		{
			name:   "ContextCanceledQC",
			err:    conn.NewQuorumCallError(context.Canceled, nil),
			target: conn.NewQuorumCallError(context.Canceled, nil),
			want:   true,
		},
		{
			name:   "ContextDeadlineExceeded",
			err:    conn.NewQuorumCallError(context.DeadlineExceeded, nil),
			target: context.DeadlineExceeded,
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Errorf("QuorumCallError.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

func TestQuorumCallErrorAccessors(t *testing.T) {
	tests := []struct {
		name           string
		qcErr          QuorumCallError
		wantCause      error
		wantNodeErrors int
	}{
		{
			name:           "NoErrors",
			qcErr:          conn.NewQuorumCallError(ErrIncomplete, nil),
			wantCause:      ErrIncomplete,
			wantNodeErrors: 0,
		},
		{
			name: "SingleError",
			qcErr: conn.NewQuorumCallError(ErrIncomplete, []conn.NodeError{
				conn.NewNodeError(1, status.Error(codes.Unavailable, "node down")),
			}),
			wantCause:      ErrIncomplete,
			wantNodeErrors: 1,
		},
		{
			name: "MultipleErrors",
			qcErr: conn.NewQuorumCallError(ErrIncomplete, []conn.NodeError{
				conn.NewNodeError(1, status.Error(codes.Unavailable, "node down")),
				conn.NewNodeError(3, status.Error(codes.DeadlineExceeded, "timeout")),
				conn.NewNodeError(5, status.Error(codes.Unavailable, "connection refused")),
			}),
			wantCause:      ErrIncomplete,
			wantNodeErrors: 3,
		},
		{
			name: "SendFailure",
			qcErr: conn.NewQuorumCallError(ErrSendFailure, []conn.NodeError{
				conn.NewNodeError(2, errors.New("send failed")),
			}),
			wantCause:      ErrSendFailure,
			wantNodeErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.qcErr.Cause(); got != tt.wantCause {
				t.Errorf("QuorumCallError.Cause() = %v, want %v", got, tt.wantCause)
			}
			if got := tt.qcErr.NumErrors(); got != tt.wantNodeErrors {
				t.Errorf("QuorumCallError.NumErrors() = %d, want %d", got, tt.wantNodeErrors)
			}
		})
	}
}

func TestQuorumCallErrorUnwrap(t *testing.T) {
	unavailableErr := status.Error(codes.Unavailable, "node down")
	timeoutErr := status.Error(codes.DeadlineExceeded, "timeout")
	connectionErr := errors.New("connection refused")

	qcErr := conn.NewQuorumCallError(ErrIncomplete, []conn.NodeError{
		conn.NewNodeError(1, unavailableErr),
		conn.NewNodeError(3, timeoutErr),
		conn.NewNodeError(5, connectionErr),
	})

	// Test Unwrap returns all node error causes
	unwrapped := qcErr.Unwrap()
	if len(unwrapped) != 3 {
		t.Fatalf("Unwrap() returned %d errors, want 3", len(unwrapped))
	}

	// Verify the unwrapped errors are the node error causes
	if unwrapped[0] != unavailableErr {
		t.Errorf("Unwrap()[0] = %v, want %v", unwrapped[0], unavailableErr)
	}
	if unwrapped[1] != timeoutErr {
		t.Errorf("Unwrap()[1] = %v, want %v", unwrapped[1], timeoutErr)
	}
	if unwrapped[2] != connectionErr {
		t.Errorf("Unwrap()[2] = %v, want %v", unwrapped[2], connectionErr)
	}

	// Test errors.Is with the cause (handled by Is() method)
	if !errors.Is(qcErr, ErrIncomplete) {
		t.Error("errors.Is(qcErr, ErrIncomplete) = false, want true")
	}

	// Test errors.Is with wrapped node errors (handled by Unwrap() method)
	if !errors.Is(qcErr, unavailableErr) {
		t.Error("errors.Is(qcErr, unavailableErr) = false, want true")
	}
	if !errors.Is(qcErr, timeoutErr) {
		t.Error("errors.Is(qcErr, timeoutErr) = false, want true")
	}
	if !errors.Is(qcErr, connectionErr) {
		t.Error("errors.Is(qcErr, connectionErr) = false, want true")
	}

	// Test errors.Is with unrelated error
	if errors.Is(qcErr, ErrSendFailure) {
		t.Error("errors.Is(qcErr, ErrSendFailure) = true, want false")
	}
}

// customError is a custom error type for testing errors.As
type customError struct {
	msg string
}

func (e customError) Error() string { return e.msg }

func TestQuorumCallErrorUnwrapWithAs(t *testing.T) {
	customErr := customError{msg: "custom node error"}
	qcErr := conn.NewQuorumCallError(ErrIncomplete, []conn.NodeError{
		conn.NewNodeError(1, customErr),
		conn.NewNodeError(2, status.Error(codes.Unavailable, "down")),
	})

	// Test errors.As can find the custom error in wrapped errors
	var target customError
	if !errors.As(qcErr, &target) {
		t.Fatal("errors.As(qcErr, &customError) = false, want true")
	}
	if target.msg != "custom node error" {
		t.Errorf("extracted customError.msg = %q, want %q", target.msg, "custom node error")
	}
}

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
