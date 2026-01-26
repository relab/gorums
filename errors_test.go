package gorums

import (
	"context"
	"errors"
	"testing"

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
			err:    QuorumCallError{cause: ErrIncomplete},
			target: ErrIncomplete,
			want:   true,
		},
		{
			name:   "SameCauseQCError",
			err:    QuorumCallError{cause: ErrIncomplete},
			target: QuorumCallError{cause: ErrIncomplete},
			want:   true,
		},
		{
			name:   "DifferentError",
			err:    QuorumCallError{cause: ErrIncomplete},
			target: errors.New("incomplete call"),
			want:   false,
		},
		{
			name:   "DifferentQCError",
			err:    QuorumCallError{cause: ErrIncomplete},
			target: QuorumCallError{cause: errors.New("incomplete call")},
			want:   false,
		},
		{
			name:   "ContextCanceled",
			err:    QuorumCallError{cause: context.Canceled},
			target: context.Canceled,
			want:   true,
		},
		{
			name:   "ContextCanceledQC",
			err:    QuorumCallError{cause: context.Canceled},
			target: QuorumCallError{cause: context.Canceled},
			want:   true,
		},
		{
			name:   "ContextDeadlineExceeded",
			err:    QuorumCallError{cause: context.DeadlineExceeded},
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
			name: "NoErrors",
			qcErr: QuorumCallError{
				cause:  ErrIncomplete,
				errors: nil,
			},
			wantCause:      ErrIncomplete,
			wantNodeErrors: 0,
		},
		{
			name: "SingleError",
			qcErr: QuorumCallError{
				cause: ErrIncomplete,
				errors: []nodeError{
					{nodeID: uint32(1), cause: status.Error(codes.Unavailable, "node down")},
				},
			},
			wantCause:      ErrIncomplete,
			wantNodeErrors: 1,
		},
		{
			name: "MultipleErrors",
			qcErr: QuorumCallError{
				cause: ErrIncomplete,
				errors: []nodeError{
					{nodeID: uint32(1), cause: status.Error(codes.Unavailable, "node down")},
					{nodeID: uint32(3), cause: status.Error(codes.DeadlineExceeded, "timeout")},
					{nodeID: uint32(5), cause: status.Error(codes.Unavailable, "connection refused")},
				},
			},
			wantCause:      ErrIncomplete,
			wantNodeErrors: 3,
		},
		{
			name: "SendFailure",
			qcErr: QuorumCallError{
				cause: ErrSendFailure,
				errors: []nodeError{
					{nodeID: uint32(2), cause: errors.New("send failed")},
				},
			},
			wantCause:      ErrSendFailure,
			wantNodeErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.qcErr.Cause(); got != tt.wantCause {
				t.Errorf("QuorumCallError.Cause() = %v, want %v", got, tt.wantCause)
			}
			if got := tt.qcErr.NodeErrors(); got != tt.wantNodeErrors {
				t.Errorf("QuorumCallError.NodeErrors() = %d, want %d", got, tt.wantNodeErrors)
			}
		})
	}
}

func TestQuorumCallErrorUnwrap(t *testing.T) {
	unavailableErr := status.Error(codes.Unavailable, "node down")
	timeoutErr := status.Error(codes.DeadlineExceeded, "timeout")
	connectionErr := errors.New("connection refused")

	qcErr := QuorumCallError{
		cause: ErrIncomplete,
		errors: []nodeError{
			{nodeID: uint32(1), cause: unavailableErr},
			{nodeID: uint32(3), cause: timeoutErr},
			{nodeID: uint32(5), cause: connectionErr},
		},
	}

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
	qcErr := QuorumCallError{
		cause: ErrIncomplete,
		errors: []nodeError{
			{nodeID: uint32(1), cause: customErr},
			{nodeID: uint32(2), cause: status.Error(codes.Unavailable, "down")},
		},
	}

	// Test errors.As can find the custom error in wrapped errors
	var target customError
	if !errors.As(qcErr, &target) {
		t.Fatal("errors.As(qcErr, &customError) = false, want true")
	}
	if target.msg != "custom node error" {
		t.Errorf("extracted customError.msg = %q, want %q", target.msg, "custom node error")
	}
}
