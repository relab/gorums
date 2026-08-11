package benchkit

import (
	"testing"
	"time"
)

// TestMeasurementAbandonStopsTicker verifies that Abandon stops the ticker's
// background goroutine instead of leaking it. Regression test for the
// pattern where MeasureLatency, ServerMeasured, and the symmetric multicast
// runner all returned early on a workload failure without stopping the
// ticker that StartMeasurement had already started; Abandon (or the
// unexported stop, for callers within benchkit) closes the ticker's done
// channel and waits for its goroutine to exit before returning.
func TestMeasurementAbandonStopsTicker(t *testing.T) {
	m := StartMeasurement(Options{Interval: time.Millisecond})
	m.Abandon()

	select {
	case _, open := <-m.ticker.done:
		if open {
			t.Error("ticker.done received a value instead of being closed")
		}
	default:
		t.Error("ticker.done is not closed; Abandon did not stop the ticker goroutine")
	}
}
