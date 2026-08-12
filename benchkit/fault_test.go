package benchkit

import (
	"testing"
	"time"
)

// TestArmFaultInjection verifies the arming guard: a non-positive duration
// disables the fault, and a positive duration returns a stoppable timer. The
// timer is stopped long before it could fire, so the test never exits.
func TestArmFaultInjection(t *testing.T) {
	if timer := ArmFaultInjection(0); timer != nil {
		timer.Stop()
		t.Error("ArmFaultInjection(0) = non-nil timer, want nil")
	}
	if timer := ArmFaultInjection(-time.Second); timer != nil {
		timer.Stop()
		t.Error("ArmFaultInjection(-1s) = non-nil timer, want nil")
	}
	timer := ArmFaultInjection(time.Hour)
	if timer == nil {
		t.Fatal("ArmFaultInjection(1h) = nil, want armed timer")
	}
	if !timer.Stop() {
		t.Error("timer already fired or stopped, want active timer")
	}
}
