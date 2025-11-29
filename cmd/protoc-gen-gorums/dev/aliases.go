package dev

import gorums "github.com/relab/gorums"

// Type aliases for important Gorums types to make them more accessible
// from user code already interacting with the generated code.
type (
	Configuration = gorums.RawConfiguration
	Manager       = gorums.RawManager
	Node          = gorums.RawNode
)

// Use the aliased types to add them to the reserved identifiers list.
// This prevents users from defining message types with these names.
var (
	_ = (*Configuration)(nil)
	_ = (*Manager)(nil)
	_ = (*Node)(nil)
)
