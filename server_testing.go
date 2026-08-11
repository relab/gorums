package gorums

import "net"

// ServerIface is implemented by servers supported by the test helpers.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}
