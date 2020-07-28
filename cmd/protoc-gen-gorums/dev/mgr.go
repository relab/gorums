package dev

import (
	"sort"

	"github.com/relab/gorums"
)

func NewManager(opts ...gorums.ManagerOption) (mgr *Manager, err error) {
	mgr = &Manager{}
	mgr.Manager, err = gorums.NewManager(orderingMethods, opts...)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

type Manager struct {
	*gorums.Manager
}

func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (*Configuration, error) {
	if len(ids) == 0 {
		return nil, gorums.IllegalConfigError("need at least one node")
	}

	var nodes []*gorums.Node
	unique := make(map[uint32]struct{})
	var uniqueIDs []uint32
	for _, nid := range ids {
		// ensure that identical IDs are only counted once
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}
		uniqueIDs = append(uniqueIDs, nid)

		node, found := m.Node(nid)
		if !found {
			return nil, gorums.NodeNotFoundError(nid)
		}

		i := sort.Search(len(nodes), func(i int) bool {
			return node.ID() < nodes[i].ID()
		})
		nodes = append(nodes, nil)
		copy(nodes[i+1:], nodes[i:])
		nodes[i] = node
	}

	c := &Configuration{
		nodes: nodes,
		n:     len(nodes),
		mgr:   m.Manager,
		qspec: qspec,
	}
	return c, nil
}
