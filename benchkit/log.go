package benchkit

import (
	"fmt"
	"os"
)

var logVerbose bool

// SetVerbose enables diagnostic output to stderr. Call once at program startup,
// before any goroutines that call Logf are started.
func SetVerbose(v bool) { logVerbose = v }

// Logf writes a formatted diagnostic message to stderr when verbose logging is
// enabled. All diagnostic output in a sweep-launched binary must use Logf rather
// than writing to os.Stdout: sweep only drains stderr, and unread stdout fills
// the SSH channel window and blocks goroutines (see the diagnostic-output rule
// in doc/benchkit-troubleshooting.html).
func Logf(format string, args ...any) {
	if logVerbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

// Printf writes a formatted diagnostic message to stderr unconditionally. Use it
// for low-volume, per-run diagnostics that must always be recorded regardless of
// -verbose (for example the clock-offset summary that documents how a
// server-measured latency was corrected), so they survive in a sweep's collected
// per-run logs. Like Logf it writes only to stderr, never stdout, which sweep
// does not drain.
func Printf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}
