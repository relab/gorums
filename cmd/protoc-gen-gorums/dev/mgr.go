package dev

import (
	"github.com/relab/gorums"
	"google.golang.org/grpc/encoding"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

func NewManager(opts ...gorums.ManagerOption) (mgr *Manager, err error) {
	mgr = &Manager{}
	mgr.Manager, err = gorums.NewManager(opts...)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

type Manager struct {
	*gorums.Manager
}

func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (c *Configuration, err error) {
	c = &Configuration{
		mgr:   m,
		qspec: qspec,
	}
	c.Configuration, err = gorums.NewConfiguration(m.Manager, ids)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.Manager.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}
