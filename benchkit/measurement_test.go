package benchkit

import (
	"slices"
	"strings"
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

// TestMeasurementAttachMetrics verifies that Attach adds the metrics of the
// client Stats to the attached result, and that it returns an error for a name
// on both sides and for a server-measured series on the client Stats, which
// has no clock offsets.
func TestMeasurementAttachMetrics(t *testing.T) {
	serverResult := func() *Result {
		return Result_builder{Metrics: []*Metric{seriesMetric("deliver", MeasurementMode_SERVER_MEASURED, 5)}}.Build()
	}
	t.Run("Union", func(t *testing.T) {
		m := StartMeasurement(Options{})
		m.Stats.Series("encode").Add(time.Microsecond)
		m.Stats.Counter("uploaded", "B").Add(10)
		r := serverResult()
		if err := m.Attach(r); err != nil {
			t.Fatalf("Attach: %v", err)
		}
		if got, want := metricNames(r), []string{"deliver", "encode", "uploaded"}; !slices.Equal(got, want) {
			t.Errorf("metric names = %v, want %v", got, want)
		}
	})
	t.Run("SameName", func(t *testing.T) {
		m := StartMeasurement(Options{})
		m.Stats.Series("deliver").Add(time.Microsecond)
		if err := m.Attach(serverResult()); err == nil || !strings.Contains(err.Error(), `"deliver"`) {
			t.Errorf("Attach = %v, want an error naming \"deliver\"", err)
		}
	})
	t.Run("ClientServerMeasured", func(t *testing.T) {
		m := StartMeasurement(Options{})
		m.Stats.Series("echo").AddBySender(1, time.Microsecond)
		if err := m.Attach(serverResult()); err == nil || !strings.Contains(err.Error(), `"echo"`) {
			t.Errorf("Attach = %v, want an error naming \"echo\"", err)
		}
	})
}
