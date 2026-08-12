package gorums

import "net"

// ServerIface is implemented by servers supported by the test helpers.
//
// The internal/testutils/servers package declares an identical interface of its
// own rather than reusing this one. That package must not import gorums, so
// that gorums's own white-box tests can use it without an import cycle, which
// leaves it no way to name this type. The two are structurally identical, so a
// value satisfying either satisfies both and no conversion is needed beyond
// restating the type.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}
