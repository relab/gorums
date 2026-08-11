package main

import (
	"testing"

	"github.com/relab/gorums"
)

// TestQuorumSize verifies that an unset -quorum-size resolves to a majority of
// the configuration, and that an explicit value is honored but clamped to the
// configuration size.
//
// The majority matters beyond arithmetic: the threshold counts the local node's
// in-process reply, so a sub-majority threshold lets a call complete without
// contacting any peer, which makes the benchmark measure local dispatch.
func TestQuorumSize(t *testing.T) {
	tests := []struct {
		name     string
		qSize    int
		numNodes int
		want     int
	}{
		{"UnsetOneNode", 0, 1, 1},
		{"UnsetThreeNodes", 0, 3, 2},
		{"UnsetFourNodes", 0, 4, 3},
		{"UnsetFiveNodes", 0, 5, 3},
		{"UnsetSevenNodes", 0, 7, 4},
		{"NegativeTreatedAsUnset", -1, 5, 3},
		{"ExplicitBelowMajority", 2, 7, 2},
		{"ExplicitAboveMajority", 6, 7, 6},
		{"ExplicitClampedToNodes", 9, 7, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &flags{qSize: tt.qSize}
			if got := f.quorumSize(tt.numNodes); got != tt.want {
				t.Errorf("quorumSize(%d) with -quorum-size=%d = %d, want %d",
					tt.numNodes, tt.qSize, got, tt.want)
			}
		})
	}
}

// TestQuorumSizeExceedsLocalReply verifies the property the default exists for:
// a majority always needs at least one reply beyond the local node's, so the
// benchmark cannot be satisfied by in-process dispatch alone.
func TestQuorumSizeExceedsLocalReply(t *testing.T) {
	f := &flags{}
	for numNodes := 2; numNodes <= 33; numNodes++ {
		if got := f.quorumSize(numNodes); got < 2 {
			t.Errorf("quorumSize(%d) = %d, want at least 2 so a peer reply is required", numNodes, got)
		}
	}
}

// TestBufferSizes verifies that an unset -send-buffer resolves to
// gorums.DefaultSendBufferSize, so recorded results show the capacity that
// actually ran instead of a zero that does not reflect it, while an explicit
// value and -recv-buffer (whose zero is already the real default) pass
// through unchanged.
func TestBufferSizes(t *testing.T) {
	tests := []struct {
		name           string
		sendBuffer     uint
		recvBuffer     uint
		wantSendBuffer uint
		wantRecvBuffer uint
	}{
		{"UnsetSendBufferResolvesToDefault", 0, 0, gorums.DefaultSendBufferSize, 0},
		{"ExplicitSendBufferPassesThrough", 4096, 0, 4096, 0},
		{"ExplicitRecvBufferPassesThrough", 0, 2048, gorums.DefaultSendBufferSize, 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &flags{sendBuffer: tt.sendBuffer, recvBuffer: tt.recvBuffer}
			gotSend, gotRecv := f.bufferSizes()
			if gotSend != tt.wantSendBuffer {
				t.Errorf("sendBuffer = %d, want %d", gotSend, tt.wantSendBuffer)
			}
			if gotRecv != tt.wantRecvBuffer {
				t.Errorf("recvBuffer = %d, want %d", gotRecv, tt.wantRecvBuffer)
			}
		})
	}
}

// TestNormalizeStreamMode verifies -stream-mode's validation and default:
// an unset value normalizes to "dual", "dedup" passes through, and any other
// value is rejected instead of silently falling back to a mode the user did
// not choose.
func TestNormalizeStreamMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    string
		wantErr bool
	}{
		{"UnsetDefaultsToDual", "", "dual", false},
		{"ExplicitDual", "dual", "dual", false},
		{"ExplicitDedup", "dedup", "dedup", false},
		{"InvalidRejected", "bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeStreamMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeStreamMode(%q) error = %v, wantErr %v", tt.mode, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("normalizeStreamMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
