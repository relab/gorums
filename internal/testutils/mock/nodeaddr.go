package mock

// NodeAddr is a node network address that implements conn.NodeAddress,
// so a map[uint32]NodeAddr can be passed to conn.WithNodes.
type NodeAddr string

// Addr returns a as a string.
func (a NodeAddr) Addr() string { return string(a) }
