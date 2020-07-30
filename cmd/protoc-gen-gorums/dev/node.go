package dev

import "github.com/relab/gorums"

type Node struct {
	*gorums.Node
	mgr *Manager
}
