package gorums

import (
	"fmt"
	"maps"
	"net"
	"slices"
)

// NodeListOption must be implemented by node providers. It is used by both the
// Manager (outbound) via newConfig and by InboundManager (inbound) via newServerConfig.
type NodeListOption interface {
	Option
	newConfig(*Manager) (Configuration, error)
	newServerConfig(*InboundManager) error
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

func (nm nodeMap[T]) newConfig(mgr *Manager) (Configuration, error) {
	if len(nm) == 0 {
		return nil, fmt.Errorf("config: missing required node map")
	}
	builder := newNodeBuilder(mgr, len(nm))
	// Sort IDs to ensure deterministic processing order
	for _, id := range slices.Sorted(maps.Keys(nm)) {
		if id == 0 {
			return nil, fmt.Errorf("config: node 0 is reserved")
		}
		node := nm[id]
		// Check if ID already exists in manager
		if _, found := mgr.Node(id); found {
			if err := builder.addExisting(id, node.Addr()); err != nil {
				return nil, err
			}
			continue
		}
		if err := builder.addNew(id, node.Addr()); err != nil {
			return nil, err
		}
	}
	return builder.configuration(), nil
}

// newServerConfig implements NodeListOption.
// It validates all IDs and addresses, then pre-creates a Node for each peer
// in im.nodes. The nodes start without an active channel; channels are
// attached by InboundManager.registerPeer when peers connect.
// Duplicate IDs are impossible (map keys are unique); duplicate addresses are
// rejected explicitly.
func (nm nodeMap[T]) newServerConfig(im *InboundManager) error {
	if len(nm) == 0 {
		return fmt.Errorf("config: missing required node map")
	}
	addrToID := make(map[string]uint32, len(nm))
	for _, id := range slices.Sorted(maps.Keys(nm)) {
		if id == 0 {
			return fmt.Errorf("config: node 0 is reserved for external clients")
		}
		addr, err := normalizeAddr(nm[id].Addr())
		if err != nil {
			return fmt.Errorf("config: invalid address %q: %w", nm[id].Addr(), err)
		}
		if existingID, exists := addrToID[addr]; exists {
			return fmt.Errorf("config: address %q already in use by node %d", addr, existingID)
		}
		addrToID[addr] = id
		im.newNode(id, addr)
	}
	return nil
}

// WithNodeList returns a NodeListOption for the provided list of node addresses.
// When used with a Manager, unique Node IDs are generated preventing conflicts
// with existing nodes. When used with NewInboundManager, IDs are assigned
// sequentially starting at 1.
func WithNodeList(addrsList []string) NodeListOption {
	return nodeList(addrsList)
}

type nodeList []string

func (nodeList) isOption() {}

func (nl nodeList) newConfig(mgr *Manager) (Configuration, error) {
	if len(nl) == 0 {
		return nil, fmt.Errorf("config: missing required node addresses")
	}
	builder := newNodeBuilder(mgr, len(nl))
	nextID := builder.nextID()
	for i, addr := range nl {
		id := nextID + uint32(i)
		if err := builder.addNew(id, addr); err != nil {
			return nil, err
		}
	}
	return builder.configuration(), nil
}

// newServerConfig implements NodeListOption for a plain address list.
// IDs are assigned sequentially starting at 1; the first address gets ID 1,
// the second ID 2, and so on.
func (nl nodeList) newServerConfig(im *InboundManager) error {
	if len(nl) == 0 {
		return fmt.Errorf("config: missing required node addresses")
	}
	addrToID := make(map[string]uint32, len(nl))
	for i, addr := range nl {
		id := uint32(i + 1)
		normalized, err := normalizeAddr(addr)
		if err != nil {
			return fmt.Errorf("config: invalid address %q: %w", addr, err)
		}
		if existingID, exists := addrToID[normalized]; exists {
			return fmt.Errorf("config: address %q already in use by node %d", normalized, existingID)
		}
		addrToID[normalized] = id
		im.newNode(id, normalized)
	}
	return nil
}

// nodeBuilder helps construct a Configuration while tracking addresses to prevent duplicates.
// It encapsulates the common logic shared between WithNodes and WithNodeList.
type nodeBuilder struct {
	mgr      *Manager
	addrToID map[string]uint32 // normalized address -> node ID
	nodes    Configuration
}

// newNodeBuilder creates a new nodeBuilder initialized with existing nodes from the manager.
func newNodeBuilder(mgr *Manager, capacity int) *nodeBuilder {
	addrToID := make(map[string]uint32, capacity)
	// Populate with existing nodes from the manager (already normalized)
	for _, existingNode := range mgr.Nodes() {
		addrToID[existingNode.Address()] = existingNode.ID()
	}
	return &nodeBuilder{
		mgr:      mgr,
		addrToID: addrToID,
		nodes:    make(Configuration, 0, capacity),
	}
}

// addExisting adds an existing node (already in manager) to the configuration.
// Returns an error if the ID exists but with a different address.
func (b *nodeBuilder) addExisting(id uint32, addr string) error {
	normalizedAddr, err := normalizeAddr(addr)
	if err != nil {
		return fmt.Errorf("config: invalid address %q: %w", addr, err)
	}
	existingNode, found := b.mgr.Node(id)
	if !found {
		return fmt.Errorf("config: node %d not found in manager", id)
	}
	if existingNode.Address() != normalizedAddr {
		return fmt.Errorf("config: node %d already in use", id)
	}
	b.nodes = append(b.nodes, existingNode)
	return nil
}

// addNew creates and adds a new node with the given ID and address.
// Returns an error if the ID is reserved (0), if the address is invalid,
// or if the normalized address is already in use.
func (b *nodeBuilder) addNew(id uint32, addr string) error {
	if id == 0 {
		return fmt.Errorf("config: node 0 is reserved")
	}
	normalizedAddr, err := normalizeAddr(addr)
	if err != nil {
		return fmt.Errorf("config: invalid address %q: %w", addr, err)
	}
	// Check for duplicate address
	if existingID, exists := b.addrToID[normalizedAddr]; exists {
		return fmt.Errorf("config: address %q already in use by node %d", normalizedAddr, existingID)
	}
	b.addrToID[normalizedAddr] = id
	node, err := b.mgr.newNode(addr, id)
	if err != nil {
		return err
	}
	b.nodes = append(b.nodes, node)
	return nil
}

// configuration returns the built Configuration, sorted by ID.
func (b *nodeBuilder) configuration() Configuration {
	OrderedBy(ID).Sort(b.mgr.nodes)
	OrderedBy(ID).Sort(b.nodes)
	return b.nodes
}

// nextID returns the next available node ID (max existing ID + 1).
func (b *nodeBuilder) nextID() uint32 {
	if nodeIDs := b.mgr.NodeIDs(); len(nodeIDs) > 0 {
		return slices.Max(nodeIDs) + 1
	}
	return 1
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
