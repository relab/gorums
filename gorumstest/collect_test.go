package gorumstest_test

import (
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums/gorumstest"
)

func TestCollectClosedChannel(t *testing.T) {
	tests := []struct {
		name  string
		send  []int
		want  int
		close bool
		got   []int
	}{
		{
			name:  "ClosedShort",
			send:  []int{1, 2},
			want:  3,
			close: true,
			got:   []int{1, 2},
		},
		{
			name:  "ClosedExact",
			send:  []int{1, 2},
			want:  2,
			close: true,
			got:   []int{1, 2},
		},
		{
			name:  "OpenExact",
			send:  []int{1, 2},
			want:  2,
			close: false,
			got:   []int{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan int, len(tt.send))
			for _, v := range tt.send {
				ch <- v
			}
			if tt.close {
				close(ch)
			}
			got := gorumstest.Collect(t, time.Second, tt.want, ch)
			if !slices.Equal(got, tt.got) {
				t.Errorf("Collect() = %v, want %v", got, tt.got)
			}
		})
	}
}
