package impl

import "github.com/relab/gorums/internal/conn"

// Aliases for the connectivity types the call engine operates on, so engine
// code reads the same as the public API in package gorums.
type (
	Node          = conn.Node
	Config        = conn.Config
	ConfigContext = conn.ConfigContext
	NodeContext   = conn.NodeContext
)
