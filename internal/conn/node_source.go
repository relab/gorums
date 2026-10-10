package conn

import (
	"fmt"
	"maps"
	"net"
	"slices"
)

// NodeSource identifies the set of nodes to build a [Config] from. Create one
// with [WithNodes] or [WithNodeList]; the interface is sealed so it can only
// be implemented within this package.
type NodeSource interface {
	newConfig(nodeRegistry) (Config, error)
}

// nodeRegistry abstracts the node management operations required to build a Config.
// Implemented by [outboundManager] and [InboundManager].
type nodeRegistry interface {
	Nodes() []*Node
	newNode(id ID, addr string) (*Node, error)
}

// NodeAddress must be implemented by types that can be used as node addresses.
type NodeAddress interface {
	Addr() string
}

// WithNodes returns a NodeSource containing the provided mapping from
// application-specific IDs to types implementing NodeAddress.
// Node IDs must be greater than 0.
func WithNodes[T NodeAddress](nodes map[ID]T) NodeSource {
	return nodeMap[T](nodes)
}

type nodeMap[T NodeAddress] map[ID]T

func (nm nodeMap[T]) newConfig(registry nodeRegistry) (Config, error) {
	if len(nm) == 0 {
		return nil, fmt.Errorf("gorums: missing required node map")
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

// WithNodeList returns a NodeSource for the provided list of node addresses.
// Unique Node IDs are generated sequentially starting from the maximum existing
// node ID plus one, or from 1 if no nodes exist, preventing conflicts with
// existing nodes.
func WithNodeList(addrsList []string) NodeSource {
	return nodeList(addrsList)
}

type nodeList []string

func (nl nodeList) newConfig(registry nodeRegistry) (Config, error) {
	if len(nl) == 0 {
		return nil, fmt.Errorf("gorums: missing required node addresses")
	}
	builder := newNodeBuilder(registry, len(nl))
	nextID := builder.nextID()
	for i, addr := range nl {
		id := nextID + ID(i)
		if err := builder.add(id, addr); err != nil {
			return nil, err
		}
	}
	return builder.configuration(), nil
}

// nodeBuilder helps construct a Config while tracking addresses to prevent duplicates.
// It encapsulates the common logic shared between WithNodes and WithNodeList.
type nodeBuilder struct {
	registry nodeRegistry
	addrToID map[string]ID // duplicate-check key (see [normalizeAddr]) -> node ID
	idToNode map[ID]*Node  // existing node ID -> node
	maxID    ID            // maximum existing node ID
	nodes    Config
}

// newNodeBuilder creates a new nodeBuilder initialized with existing nodes from the registry.
func newNodeBuilder(registry nodeRegistry, capacity int) *nodeBuilder {
	addrToID := make(map[string]ID, capacity)
	idToNode := make(map[ID]*Node, capacity)
	maxID := ID(0)
	// Populate with existing nodes from the registry
	for _, existingNode := range registry.Nodes() {
		id := existingNode.ID()
		addrToID[addrKey(existingNode.Address())] = id
		idToNode[id] = existingNode
		maxID = max(maxID, id)
	}
	return &nodeBuilder{
		registry: registry,
		addrToID: addrToID,
		idToNode: idToNode,
		maxID:    maxID,
		nodes:    make(Config, 0, capacity),
	}
}

// add creates or reuses a node with the given ID and address.
func (b *nodeBuilder) add(id ID, addr string) error {
	if id == 0 {
		return fmt.Errorf("gorums: node 0 is reserved")
	}
	key, err := normalizeAddr(addr)
	if err != nil {
		return fmt.Errorf("gorums: invalid address %q: %w", addr, err)
	}

	// If ID already exists, verify address matches
	if existingNode, found := b.idToNode[id]; found {
		if addrKey(existingNode.Address()) != key {
			return fmt.Errorf("gorums: node %d already in use by %q", id, existingNode.Address())
		}
		b.nodes = append(b.nodes, existingNode)
		return nil
	}

	// Check for duplicate address
	if existingID, exists := b.addrToID[key]; exists {
		return fmt.Errorf("gorums: address %q already in use by node %d", key, existingID)
	}

	b.addrToID[key] = id
	node, err := b.registry.newNode(id, addr)
	if err != nil {
		return err
	}
	b.nodes = append(b.nodes, node)
	return nil
}

// configuration returns the built Config, sorted by ID.
func (b *nodeBuilder) configuration() Config {
	slices.SortFunc(b.nodes, ByID)
	return b.nodes
}

// nextID returns the next available node ID (max existing ID + 1).
func (b *nodeBuilder) nextID() ID {
	return b.maxID + 1
}

// normalizeAddr resolves addr with net.ResolveTCPAddr to a canonical form that
// serves only as a duplicate-detection key; nodes keep the configured address.
// For example, "localhost:8080" and "127.0.0.1:8080" may resolve to the same key.
func normalizeAddr(addr string) (string, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return "", err
	}
	return tcpAddr.String(), nil
}

// addrKey returns the duplicate-detection key for addr, or addr itself if it
// does not resolve.
func addrKey(addr string) string {
	if key, err := normalizeAddr(addr); err == nil {
		return key
	}
	return addr
}
