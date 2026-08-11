package benchkit

import (
	"os"
	"time"
)

// ArmFaultInjection schedules an abrupt, unannounced process exit after d,
// simulating a node crash mid-run for fault-injection experiments (the
// -fault-kill-after flag, see StandardFlags). The exit is clean (status 0) so
// remote launchers do not flag it; the node simply writes no result file, and
// sweep reports the missing file and summarizes the surviving nodes. A
// non-positive d disables the fault and returns nil; otherwise the returned
// timer can stop the scheduled exit (used by tests).
func ArmFaultInjection(d time.Duration) *time.Timer {
	if d <= 0 {
		return nil
	}
	return time.AfterFunc(d, func() {
		Logf("fault injection: exiting after %v\n", d)
		os.Exit(0) // skipcq: RVV-A0003
	})
}
