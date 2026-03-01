package gorums

import (
	"fmt"
	"maps"
	"net"
	"slices"
)

// NodeListOption must be implemented by node providers. It is used by both the
// Manager (outbound) and by InboundManager (inbound) via newConfig.
type NodeListOption interface {
	Option
	newConfig(nodeRegistry) (Configuration, error)
}

// nodeRegistry abstracts the node management operations required to build a Configuration.
// Implemented by Manager and InboundManager.
type nodeRegistry interface {
	Nodes() []*Node
	newNode(id uint32, addr string) (*Node, error)
}

// NodeAddress must be implemented by types that can be used as node addresses.
type NodeAddress interface {
	Addr() string
}

// WithNodes returns a NodeListOption containing the provided mapping from
// application-specific IDs to types implementing NodeAddress.
// Node IDs must be greater than 0.
func WithNodes[T NodeAddress](nodes map[uint32]T) NodeListOption {
	return nodeMap[T](nodes)
}

type nodeMap[T NodeAddress] map[uint32]T

func (nodeMap[T]) isOption() {}

func (nm nodeMap[T]) newConfig(registry nodeRegistry) (Configuration, error) {
	if len(nm) == 0 {
		return nil, fmt.Errorf("config: missing required node map")
	}
	builder := newNodeBuilder(registry, len(nm))
	// Sort IDs to ensure deterministic processing order
	for _, id := range slices.Sorted(maps.Keys(nm)) {
		node := nm[id]
		if err := builder.add(id, node.Addr()); err != nil {
			return nil, err
		}
	}
	return builder.configuration(), nil
}

// WithNodeList returns a NodeListOption for the provided list of node addresses.
// Unique Node IDs are generated sequentially starting from the maximum existing
// node ID plus one, or from 1 if no nodes exist, preventing conflicts with
// existing nodes.
func WithNodeList(addrsList []string) NodeListOption {
	return nodeList(addrsList)
}

type nodeList []string

func (nodeList) isOption() {}

func (nl nodeList) newConfig(registry nodeRegistry) (Configuration, error) {
	if len(nl) == 0 {
		return nil, fmt.Errorf("config: missing required node addresses")
	}
	builder := newNodeBuilder(registry, len(nl))
	nextID := builder.nextID()
	for i, addr := range nl {
		id := nextID + uint32(i)
		if err := builder.add(id, addr); err != nil {
			return nil, err
		}
	}
	return builder.configuration(), nil
}

// nodeBuilder helps construct a Configuration while tracking addresses to prevent duplicates.
// It encapsulates the common logic shared between WithNodes and WithNodeList.
type nodeBuilder struct {
	registry nodeRegistry
	addrToID map[string]uint32 // normalized address -> node ID
	idToNode map[uint32]*Node  // existing node ID -> node
	maxID    uint32            // maximum existing node ID
	nodes    Configuration
}

// newNodeBuilder creates a new nodeBuilder initialized with existing nodes from the registry.
func newNodeBuilder(registry nodeRegistry, capacity int) *nodeBuilder {
	addrToID := make(map[string]uint32, capacity)
	idToNode := make(map[uint32]*Node, capacity)
	maxID := uint32(0)
	// Populate with existing nodes from the registry (already normalized)
	for _, existingNode := range registry.Nodes() {
		id := existingNode.ID()
		addrToID[existingNode.Address()] = id
		idToNode[id] = existingNode
		maxID = max(maxID, id)
	}
	return &nodeBuilder{
		registry: registry,
		addrToID: addrToID,
		idToNode: idToNode,
		maxID:    maxID,
		nodes:    make(Configuration, 0, capacity),
	}
}

// add creates or reuses a node with the given ID and address.
func (b *nodeBuilder) add(id uint32, addr string) error {
	if id == 0 {
		return fmt.Errorf("config: node 0 is reserved")
	}
	normalizedAddr, err := normalizeAddr(addr)
	if err != nil {
		return fmt.Errorf("config: invalid address %q: %w", addr, err)
	}

	// If ID already exists, verify address matches
	if existingNode, found := b.idToNode[id]; found {
		if existingNode.Address() != normalizedAddr {
			return fmt.Errorf("config: node %d already in use by %q", id, existingNode.Address())
		}
		b.nodes = append(b.nodes, existingNode)
		return nil
	}

	// Check for duplicate address
	if existingID, exists := b.addrToID[normalizedAddr]; exists {
		return fmt.Errorf("config: address %q already in use by node %d", normalizedAddr, existingID)
	}

	b.addrToID[normalizedAddr] = id
	node, err := b.registry.newNode(id, addr)
	if err != nil {
		return err
	}
	b.nodes = append(b.nodes, node)
	return nil
}

// configuration returns the built Configuration, sorted by ID.
func (b *nodeBuilder) configuration() Configuration {
	OrderedBy(ID).Sort(b.nodes)
	return b.nodes
}

// nextID returns the next available node ID (max existing ID + 1).
func (b *nodeBuilder) nextID() uint32 {
	return b.maxID + 1
}

// normalizeAddr normalizes an address string to a canonical form using
// net.ResolveTCPAddr. This ensures consistent address comparison for
// duplicate detection. For example, "localhost:8080" and "127.0.0.1:8080"
// may resolve to the same normalized address.
func normalizeAddr(addr string) (string, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return "", err
	}
	return tcpAddr.String(), nil
}
