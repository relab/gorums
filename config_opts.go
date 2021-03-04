package gorums

import "fmt"

type ConfigOption interface{}

type NodeListOption interface {
	ConfigOption
	newConfig(*Manager) (Configuration, error)
}

type nodeIDMap struct {
	idMap map[string]uint32
}

func (o nodeIDMap) newConfig(mgr *Manager) (nodes Configuration, err error) {
	if len(o.idMap) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node-to-ID map required: WithNodeMap"))
	}
	nodes = make(Configuration, 0)
	for naddr, id := range o.idMap {
		node, found := mgr.Node(id)
		if !found {
			node, err = NewNodeWithID(naddr, id)
			if err != nil {
				return nil, ConfigCreationError(err)
			}
			err = mgr.AddNode(node)
			if err != nil {
				return nil, ConfigCreationError(err)
			}
		}
		nodes = append(nodes, node)
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, nil
}

func WithNodeMap(idMap map[string]uint32) NodeListOption {
	return &nodeIDMap{idMap: idMap}
}

type nodeList struct {
	addrsList []string
}

func (o nodeList) newConfig(mgr *Manager) (Configuration, error) {
	if len(o.addrsList) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node addresses required: WithNodeList"))
	}
	nodes := make(Configuration, 0)
	for _, naddr := range o.addrsList {
		node, err := NewNode(naddr)
		if err != nil {
			return nil, ConfigCreationError(err)
		}
		if n, found := mgr.Node(node.ID()); !found {
			err = mgr.AddNode(node)
			if err != nil {
				return nil, ConfigCreationError(err)
			}
		} else {
			node = n
		}
		nodes = append(nodes, node)
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, nil
}

func WithNodeList(addrsList []string) NodeListOption {
	return &nodeList{addrsList: addrsList}
}

type nodeIDs struct {
	nodeIDs []uint32
}

func (o nodeIDs) newConfig(mgr *Manager) (Configuration, error) {
	if len(o.nodeIDs) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node IDs required: WithNodeIDs"))
	}
	nodes := make(Configuration, 0)
	for _, id := range o.nodeIDs {
		node, found := mgr.Node(id)
		if !found {
			// Node IDs must have been registered previously
			return nil, ConfigCreationError(fmt.Errorf("node ID %d not found", id))
		}
		nodes = append(nodes, node)
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, nil
}

func WithNodeIDs(ids []uint32) NodeListOption {
	return &nodeIDs{nodeIDs: ids}
}
