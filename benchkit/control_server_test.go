package benchkit

import (
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums"
)

// TestControlStartHonorsStatsMode verifies that the Start RPC builds the server's
// aggregate store in the mode carried by StartRequest, so a server-measured HDR
// run returns a histogram from Stop, and that a later Start with a different mode
// reconfigures the same Control.
func TestControlStartHonorsStatsMode(t *testing.T) {
	ctrl := NewControl()

	if _, err := ctrl.Start(gorums.ServerContext{}, StartRequest_builder{StatsMode: StatsMode_HDR}.Build()); err != nil {
		t.Fatalf("Start(HDR): %v", err)
	}
	ctrl.Stats().AddLatency(5 * time.Microsecond)
	r, err := ctrl.Stop(gorums.ServerContext{}, &StopRequest{})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := r.GetLatencies(); got != nil {
		t.Errorf("Latencies in HDR mode = %v, want nil", got)
	}
	if r.GetHistogram() == nil {
		t.Error("Histogram in HDR mode = nil, want non-nil")
	}

	// The zero-value StartRequest selects StatsMode_EXACT, reconfiguring the
	// same Control back to raw-sample storage.
	if _, err := ctrl.Start(gorums.ServerContext{}, &StartRequest{}); err != nil {
		t.Fatalf("Start(EXACT): %v", err)
	}
	ctrl.Stats().AddLatency(5 * time.Microsecond)
	r2, err := ctrl.Stop(gorums.ServerContext{}, &StopRequest{})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := r2.GetLatencies(); len(got) != 1 {
		t.Errorf("Latencies after EXACT restart = %v, want one sample", got)
	}
	if r2.GetHistogram() != nil {
		t.Error("Histogram after EXACT restart != nil, want nil")
	}
}

func TestControlDoneCount(t *testing.T) {
	ctrl := NewControl()
	if got := ctrl.DoneCount(); got != 0 {
		t.Errorf("DoneCount before ArmDone = %d, want 0", got)
	}
	ctrl.ArmDone(3)
	if got := ctrl.DoneCount(); got != 0 {
		t.Errorf("DoneCount after ArmDone = %d, want 0", got)
	}
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 2}.Build())
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 2}.Build()) // duplicate: counted once
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 3}.Build())
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 9}.Build()) // out of range: ignored
	if got := ctrl.DoneCount(); got != 2 {
		t.Errorf("DoneCount = %d, want 2", got)
	}
}

func TestControlDoneClosesChannelWhenAllSendersSignal(t *testing.T) {
	ctrl := NewControl()
	doneCh := ctrl.ArmDone(3)

	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 1}.Build())
	select {
	case <-doneCh:
		t.Fatal("DoneCh closed after 1/3 signals, want open")
	default:
	}

	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 2}.Build())
	select {
	case <-doneCh:
		t.Fatal("DoneCh closed after 2/3 signals, want open")
	default:
	}

	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 3}.Build())
	select {
	case <-doneCh:
	default:
		t.Fatal("DoneCh open after 3/3 signals, want closed")
	}
}

func TestControlDoneIgnoresDuplicateSender(t *testing.T) {
	ctrl := NewControl()
	doneCh := ctrl.ArmDone(2)

	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 1}.Build())
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 1}.Build())
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 1}.Build())

	select {
	case <-doneCh:
		t.Fatal("DoneCh closed after repeated signals from one sender, want open")
	default:
	}

	if got := ctrl.MissingDone(); !slices.Equal(got, []uint32{2}) {
		t.Errorf("MissingDone() = %v, want [2]", got)
	}
}

func TestControlDoneTracksMissingSenders(t *testing.T) {
	ctrl := NewControl()
	ctrl.ArmDone(3)

	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 2}.Build())

	if got := ctrl.MissingDone(); !slices.Equal(got, []uint32{1, 3}) {
		t.Errorf("MissingDone() = %v, want [1 3]", got)
	}
}

func TestControlDoneNoopWhenUnarmed(t *testing.T) {
	ctrl := NewControl()

	if ctrl.DoneCh() != nil {
		t.Fatal("DoneCh() != nil before ArmDone, want nil")
	}

	// Must not panic.
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 1}.Build())

	if ctrl.DoneCh() != nil {
		t.Fatal("DoneCh() != nil after Done() without ArmDone, want nil")
	}
}

func TestControlDoneIgnoresSenderIDOutOfRange(t *testing.T) {
	ctrl := NewControl()
	doneCh := ctrl.ArmDone(2)

	// sender_id 0 and out-of-range IDs must not panic or count toward the total.
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 0}.Build())
	ctrl.Done(gorums.ServerContext{}, DoneRequest_builder{SenderId: 99}.Build())

	select {
	case <-doneCh:
		t.Fatal("DoneCh closed after out-of-range senders, want open")
	default:
	}
	if got := ctrl.MissingDone(); !slices.Equal(got, []uint32{1, 2}) {
		t.Errorf("MissingDone() = %v, want [1 2]", got)
	}
}
