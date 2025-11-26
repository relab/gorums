package gorums

import (
	"context"
	"errors"
	"testing"
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
