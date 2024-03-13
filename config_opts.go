package gorums

import "fmt"

// ConfigOption is a marker interface for options to NewConfiguration.
type ConfigOption interface{}

// NodeListOption must be implemented by node providers.
type NodeListOption interface {
	ConfigOption
	newConfig(*RawManager) (RawConfiguration, error)
}

type nodeIDMap struct {
	idMap map[string]uint32
}

func (o nodeIDMap) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	if len(o.idMap) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node-to-ID map required: WithNodeMap"))
	}
	nodes = make(RawConfiguration, 0, len(o.idMap))
	for naddr, id := range o.idMap {
		node, found := mgr.Node(id)
		if !found {
			node, err = NewRawNodeWithID(naddr, id)
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

// WithNodeMap returns a NodeListOption containing the provided
// mapping from node addresses to application-specific IDs.
func WithNodeMap(idMap map[string]uint32) NodeListOption {
	return &nodeIDMap{idMap: idMap}
}

type nodeList struct {
	addrsList []string
}

func (o nodeList) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	if len(o.addrsList) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node addresses required: WithNodeList"))
	}
	nodes = make(RawConfiguration, 0, len(o.addrsList))
	for _, naddr := range o.addrsList {
		node, err := NewRawNode(naddr)
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

// WithNodeList returns a NodeListOption containing the provided list of node addresses.
// With this option, node IDs are generated by the Manager.
func WithNodeList(addrsList []string) NodeListOption {
	return &nodeList{addrsList: addrsList}
}

type nodeIDs struct {
	nodeIDs []uint32
}

func (o nodeIDs) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	if len(o.nodeIDs) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node IDs required: WithNodeIDs"))
	}
	nodes = make(RawConfiguration, 0, len(o.nodeIDs))
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

// WithNodeIDs returns a NodeListOption containing a list of node IDs.
// This assumes that the provided node IDs have already been registered with the manager.
func WithNodeIDs(ids []uint32) NodeListOption {
	return &nodeIDs{nodeIDs: ids}
}

type addNodes struct {
	old RawConfiguration
	new NodeListOption
}

func (o addNodes) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	newNodes, err := o.new.newConfig(mgr)
	if err != nil {
		return nil, err
	}
	ac := &addConfig{old: o.old, add: newNodes}
	return ac.newConfig(mgr)
}

// WithNewNodes returns a NodeListOption that can be used to create a new configuration
// combining c and the new nodes.
func (c RawConfiguration) WithNewNodes(new NodeListOption) NodeListOption {
	return &addNodes{old: c, new: new}
}

type addConfig struct {
	old RawConfiguration
	add RawConfiguration
}

func (o addConfig) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	nodes = make(RawConfiguration, 0, len(o.old)+len(o.add))
	m := make(map[uint32]bool)
	for _, n := range append(o.old, o.add...) {
		if !m[n.id] {
			m[n.id] = true
			nodes = append(nodes, n)
		}
	}
	// Sort nodes to ensure deterministic iteration.
	OrderedBy(ID).Sort(mgr.nodes)
	OrderedBy(ID).Sort(nodes)
	return nodes, err
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c RawConfiguration) And(d RawConfiguration) NodeListOption {
	return &addConfig{old: c, add: d}
}

// WithoutNodes returns a NodeListOption that can be used to create a new configuration
// from c without the given node IDs.
func (c RawConfiguration) WithoutNodes(ids ...uint32) NodeListOption {
	rmIDs := make(map[uint32]bool)
	for _, id := range ids {
		rmIDs[id] = true
	}
	keepIDs := make([]uint32, 0, len(c))
	for _, cNode := range c {
		if !rmIDs[cNode.id] {
			keepIDs = append(keepIDs, cNode.id)
		}
	}
	return &nodeIDs{nodeIDs: keepIDs}
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c RawConfiguration) Except(rm RawConfiguration) NodeListOption {
	rmIDs := make(map[uint32]bool)
	for _, rmNode := range rm {
		rmIDs[rmNode.id] = true
	}
	keepIDs := make([]uint32, 0, len(c))
	for _, cNode := range c {
		if !rmIDs[cNode.id] {
			keepIDs = append(keepIDs, cNode.id)
		}
	}
	return &nodeIDs{nodeIDs: keepIDs}
}

type nodeListBroadcast struct {
	addrsList []string
}

func (o nodeListBroadcast) newConfig(mgr *RawManager) (nodes RawConfiguration, err error) {
	if len(o.addrsList) == 0 {
		return nil, ConfigCreationError(fmt.Errorf("node addresses required: WithNodeList"))
	}
	nodes = make(RawConfiguration, 0, len(o.addrsList))
	for _, naddr := range o.addrsList {
		node, err := NewRawNode(naddr)
		if err != nil {
			return nil, ConfigCreationError(err)
		}
		if n, found := mgr.Node(node.ID()); !found {
			err := mgr.AddNode(node)
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
	return nodes, err
}

// WithNodeList returns a NodeListOption containing the provided list of node addresses.
// With this option, node IDs are generated by the Manager.
func WithNodeListBroadcast(addrsList []string) NodeListOption {
	return &nodeListBroadcast{addrsList: addrsList}
}
