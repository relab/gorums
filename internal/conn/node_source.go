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
	// nodeMap returns the captured, explicitly numbered node records, or nil
	// for a source whose IDs are assigned when the configuration is built.
	nodeMap() nodeMap
}

// nodeRegistry abstracts the node management operations required to build a Config.
// Implemented by [outboundManager] and [InboundManager].
type nodeRegistry interface {
	Nodes() []*Node
	// newNode creates the node with the given ID. A non-nil verify checks the
	// remote end of each stream that the node dials.
	newNode(id ID, addr string, verify PeerVerifier) (*Node, error)
}

// NodeAddress must be implemented by types that can be used as node addresses.
type NodeAddress interface {
	Addr() string
}

// TLSIdentity supplies the expected URI SAN or DNS SAN of a peer's verified
// TLS certificate. A node source captures it alongside the node's address.
type TLSIdentity interface {
	TLSIdentity() string
}

// WithNodes returns a NodeSource containing the provided mapping from
// application-specific IDs to types implementing NodeAddress.
// Node IDs must be greater than 0. It captures each node's address and, when
// the node implements [TLSIdentity], its expected certificate identity.
func WithNodes[T NodeAddress](nodes map[ID]T) NodeSource {
	nm := make(nodeMap, len(nodes))
	for _, id := range slices.Sorted(maps.Keys(nodes)) {
		node := nodes[id]
		addr := nodeAddress{addr: node.Addr()}
		if identity, ok := any(node).(TLSIdentity); ok {
			addr.identity = identity.TLSIdentity()
			addr.hasIdentity = true
		}
		nm[id] = addr
	}
	return nm
}

// nodeAddress holds a snapshot of a node's address and optional TLS identity.
type nodeAddress struct {
	addr        string
	identity    string
	hasIdentity bool
}

type nodeMap map[ID]nodeAddress

func (nm nodeMap) nodeMap() nodeMap { return nm }

// hasIdentity reports whether any node declared [TLSIdentity], even if empty.
func (nm nodeMap) hasIdentity() bool {
	for _, node := range nm {
		if node.hasIdentity {
			return true
		}
	}
	return false
}

func (nm nodeMap) newConfig(registry nodeRegistry) (Config, error) {
	if len(nm) == 0 {
		return nil, fmt.Errorf("gorums: missing required node map")
	}
	// Nodes that declare an identity check the servers they dial.
	verify, err := NewPeerVerifier(nm)
	if err != nil {
		return nil, err
	}
	builder := newNodeBuilder(registry, len(nm), verify)
	// Sort IDs to ensure deterministic processing order
	for _, id := range slices.Sorted(maps.Keys(nm)) {
		node := nm[id]
		if err := builder.add(id, node.addr); err != nil {
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

func (nl nodeList) nodeMap() nodeMap { return nil }

func (nl nodeList) newConfig(registry nodeRegistry) (Config, error) {
	if len(nl) == 0 {
		return nil, fmt.Errorf("gorums: missing required node addresses")
	}
	builder := newNodeBuilder(registry, len(nl), nil)
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
	verify   PeerVerifier  // optional; checks the remote end of each node's streams
	addrToID map[string]ID // resolved address -> node ID
	idToNode map[ID]*Node  // existing node ID -> node
	maxID    ID            // maximum existing node ID
	nodes    Config
}

// newNodeBuilder creates a new nodeBuilder initialized with existing nodes from the registry.
func newNodeBuilder(registry nodeRegistry, capacity int, verify PeerVerifier) *nodeBuilder {
	addrToID := make(map[string]ID, capacity)
	idToNode := make(map[ID]*Node, capacity)
	maxID := ID(0)
	// Populate with existing nodes from the registry
	for _, existingNode := range registry.Nodes() {
		id := existingNode.ID()
		addrToID[comparableAddr(existingNode.Address())] = id
		idToNode[id] = existingNode
		maxID = max(maxID, id)
	}
	return &nodeBuilder{
		registry: registry,
		verify:   verify,
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
	resolved, err := resolveAddr(addr)
	if err != nil {
		return fmt.Errorf("gorums: invalid address %q: %w", addr, err)
	}

	// If ID already exists, verify address matches
	if existingNode, found := b.idToNode[id]; found {
		if comparableAddr(existingNode.Address()) != resolved {
			return fmt.Errorf("gorums: node %d already in use by %q", id, existingNode.Address())
		}
		b.nodes = append(b.nodes, existingNode)
		return nil
	}

	// Check for duplicate address
	if existingID, exists := b.addrToID[resolved]; exists {
		return fmt.Errorf("gorums: address %q already in use by node %d", resolved, existingID)
	}

	b.addrToID[resolved] = id
	node, err := b.registry.newNode(id, addr, b.verify)
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

// resolveAddr returns the canonical form of addr, with any hostname resolved
// to an IP address, for example "localhost:8080" becomes "127.0.0.1:8080".
// It is used to compare addresses; nodes keep their configured address.
// An invalid address returns an error.
func resolveAddr(addr string) (string, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return "", err
	}
	return tcpAddr.String(), nil
}

// comparableAddr returns the resolved form of addr, or addr itself if it does
// not resolve.
func comparableAddr(addr string) string {
	if resolved, err := resolveAddr(addr); err == nil {
		return resolved
	}
	return addr
}
