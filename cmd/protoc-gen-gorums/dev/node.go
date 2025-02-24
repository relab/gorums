package dev

import (
	"cmp"

	"github.com/relab/gorums"
)

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node[idType cmp.Ordered] struct {
	*gorums.RawNode[idType]
}
