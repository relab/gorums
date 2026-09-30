package gorums

import "net"

// ServerIface is implemented by servers supported by the test helpers.
//
// Package [github.com/relab/gorums/internal/testutils/servers] declares a
// structurally identical interface, since it does not import gorums; a value
// satisfying either satisfies both.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}
