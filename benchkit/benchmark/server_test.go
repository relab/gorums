package benchmark

import (
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/gorumstest"
)

// TestServerMulticastBucketsBySender verifies that Multicast records a sample
// into the bucket of the sender that ServerContext.SenderID reports, so
// GetResultCorrected shifts it by that sender's offset, and that a request
// from a sender with ID 0 records into the flat samples instead.
func TestServerMulticastBucketsBySender(t *testing.T) {
	const offset = int64(time.Hour) // far above any real latency
	offsets := map[uint32]int64{2: offset}

	tests := []struct {
		name          string
		senderID      uint32
		wantCorrected int // samples that GetResultCorrected returns
		wantFlat      int // samples that GetResult returns
	}{
		{name: "known sender", senderID: 2, wantCorrected: 1, wantFlat: 0},
		{name: "regular client", senderID: 0, wantCorrected: 0, wantFlat: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := benchkit.NewControl()
			srv := &workloadServer{ctrl: ctrl}
			ctrl.Stats().Start()

			msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
			srv.Multicast(gorumstest.ServerContext(t, tt.senderID), msg)
			ctrl.Stats().End()

			corrected := ctrl.Stats().GetResultCorrected(offsets).GetLatencies()
			if len(corrected) != tt.wantCorrected {
				t.Fatalf("GetResultCorrected samples = %d, want %d", len(corrected), tt.wantCorrected)
			}
			if tt.wantCorrected > 0 && corrected[0] < offset {
				t.Errorf("corrected sample = %d, want at least the offset %d", corrected[0], offset)
			}
			if got := len(ctrl.Stats().GetResult().GetLatencies()); got != tt.wantFlat {
				t.Errorf("GetResult samples = %d, want %d", got, tt.wantFlat)
			}
		})
	}
}
