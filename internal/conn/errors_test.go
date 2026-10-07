package conn

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Package conn cannot import the call engine's sentinel errors, so these
// stand in for them as quorum call causes.
var (
	errIncomplete  = errors.New("incomplete call")
	errSendFailure = errors.New("send failure")
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
			err:    NewQuorumCallError(errIncomplete, nil),
			target: errIncomplete,
			want:   true,
		},
		{
			name:   "SameCauseQCError",
			err:    NewQuorumCallError(errIncomplete, nil),
			target: NewQuorumCallError(errIncomplete, nil),
			want:   true,
		},
		{
			name:   "DifferentError",
			err:    NewQuorumCallError(errIncomplete, nil),
			target: errors.New("incomplete call"),
			want:   false,
		},
		{
			name:   "DifferentQCError",
			err:    NewQuorumCallError(errIncomplete, nil),
			target: NewQuorumCallError(errors.New("incomplete call"), nil),
			want:   false,
		},
		{
			name:   "ContextCanceled",
			err:    NewQuorumCallError(context.Canceled, nil),
			target: context.Canceled,
			want:   true,
		},
		{
			name:   "ContextCanceledQC",
			err:    NewQuorumCallError(context.Canceled, nil),
			target: NewQuorumCallError(context.Canceled, nil),
			want:   true,
		},
		{
			name:   "ContextDeadlineExceeded",
			err:    NewQuorumCallError(context.DeadlineExceeded, nil),
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
			qcErr:          NewQuorumCallError(errIncomplete, nil),
			wantCause:      errIncomplete,
			wantNodeErrors: 0,
		},
		{
			name: "SingleError",
			qcErr: NewQuorumCallError(errIncomplete, []NodeError{
				NewNodeError(1, status.Error(codes.Unavailable, "node down")),
			}),
			wantCause:      errIncomplete,
			wantNodeErrors: 1,
		},
		{
			name: "MultipleErrors",
			qcErr: NewQuorumCallError(errIncomplete, []NodeError{
				NewNodeError(1, status.Error(codes.Unavailable, "node down")),
				NewNodeError(3, status.Error(codes.DeadlineExceeded, "timeout")),
				NewNodeError(5, status.Error(codes.Unavailable, "connection refused")),
			}),
			wantCause:      errIncomplete,
			wantNodeErrors: 3,
		},
		{
			name: "SendFailure",
			qcErr: NewQuorumCallError(errSendFailure, []NodeError{
				NewNodeError(2, errors.New("send failed")),
			}),
			wantCause:      errSendFailure,
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

	qcErr := NewQuorumCallError(errIncomplete, []NodeError{
		NewNodeError(1, unavailableErr),
		NewNodeError(3, timeoutErr),
		NewNodeError(5, connectionErr),
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
	if !errors.Is(qcErr, errIncomplete) {
		t.Error("errors.Is(qcErr, errIncomplete) = false, want true")
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
	if errors.Is(qcErr, errSendFailure) {
		t.Error("errors.Is(qcErr, errSendFailure) = true, want false")
	}
}

// customError is a custom error type for testing errors.As
type customError struct {
	msg string
}

func (e customError) Error() string { return e.msg }

func TestQuorumCallErrorUnwrapWithAs(t *testing.T) {
	customErr := customError{msg: "custom node error"}
	qcErr := NewQuorumCallError(errIncomplete, []NodeError{
		NewNodeError(1, customErr),
		NewNodeError(2, status.Error(codes.Unavailable, "down")),
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
