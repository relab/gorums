package gorums

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ConfigContext is a context that carries a configuration for quorum calls.
// It embeds context.Context and provides access to the Configuration.
//
// Use [Configuration.Context] to create a ConfigContext from an existing context.
type ConfigContext struct {
	context.Context
	cfg Configuration
}

// Configuration returns the Configuration associated with this context.
func (c ConfigContext) Configuration() Configuration {
	return c.cfg
}

// Configuration represents a static set of nodes on which quorum calls may be invoked.
// A configuration is created using [NewConfiguration] or [NewConfig]. A configuration
// should be treated as immutable. Therefore, methods that operate on a configuration
// always return a new Configuration instance.
type Configuration []*Node

// Context creates a new ConfigContext from the given parent context
// and this configuration.
//
// Example:
//
//	config, _ := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
//	cfgCtx := config.Context(context.Background())
//	resp, err := paxos.Prepare(cfgCtx, req)
func (c Configuration) Context(parent context.Context) *ConfigContext {
	if len(c) == 0 {
		panic("gorums: Context called with empty configuration")
	}
	return &ConfigContext{Context: parent, cfg: c}
}

// NewConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodes or WithNodeList.
// A new configuration can also be created from an existing configuration
// using the Add, Union, Remove, Difference, Extend, and WithoutErrors methods.
func NewConfiguration(mgr *Manager, opt NodeListOption) (nodes Configuration, err error) {
	if opt == nil {
		return nil, fmt.Errorf("config: missing required node list")
	}
	return opt.newConfig(mgr)
}

// NewConfig returns a new [Configuration] based on the provided [gorums.Option]s.
// It accepts exactly one [gorums.NodeListOption] and multiple [gorums.ManagerOption]s.
// You may use this function to create the initial configuration for a new manager.
//
// Example:
//
//		cfg, err := NewConfig(
//		    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	        gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//		)
//
// This is a convenience function for creating a configuration without explicitly
// creating a manager first. However, the manager can be accessed using the
// [Configuration.Manager] method. This method should only be used once since it
// creates a new manager; if a manager already exists, use [NewConfiguration]
// instead, and provide the existing manager as the first argument.
func NewConfig(opts ...Option) (Configuration, error) {
	var (
		managerOptions []ManagerOption
		nodeListOption NodeListOption
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case ManagerOption:
			managerOptions = append(managerOptions, o)
		case NodeListOption:
			if nodeListOption != nil {
				return nil, fmt.Errorf("gorums: multiple NodeListOptions provided")
			}
			nodeListOption = o
		default:
			return nil, fmt.Errorf("gorums: unknown option type: %T", opt)
		}
	}
	if nodeListOption == nil {
		return nil, fmt.Errorf("gorums: missing required NodeListOption")
	}
	mgr := NewManager(managerOptions...)
	return NewConfiguration(mgr, nodeListOption)
}

// Extend returns a new Configuration combining c with new nodes from the provided NodeListOption.
// This is the only way to add nodes that are not yet registered with the manager.
func (c Configuration) Extend(opt NodeListOption) (Configuration, error) {
	if len(c) == 0 {
		return nil, fmt.Errorf("config: cannot extend empty configuration")
	}
	if opt == nil {
		return slices.Clone(c), nil
	}
	mgr := c.Manager()
	newNodes, err := opt.newConfig(mgr)
	if err != nil {
		return nil, err
	}
	return c.Union(newNodes), nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c Configuration) Nodes() []*Node {
	return c
}

// Size returns the number of nodes in this configuration.
func (c Configuration) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c Configuration) Equal(b Configuration) bool {
	if len(c) != len(b) {
		return false
	}
	for i := range c {
		if c[i].ID() != b[i].ID() {
			return false
		}
	}
	return true
}

// Manager returns the Manager that manages this configuration's nodes.
// Returns nil if the configuration is empty.
func (c Configuration) Manager() *Manager {
	if len(c) == 0 {
		return nil
	}
	return c[0].mgr
}

// nextMsgID returns the next message ID from this client's manager.
func (c Configuration) nextMsgID() uint64 {
	return c[0].msgIDGen()
}

// Contains reports whether c contains a node with the given ID.
func (c Configuration) Contains(id uint32) bool {
	return slices.ContainsFunc(c, func(n *Node) bool { return n.id == id })
}

// Add returns a new Configuration containing nodes from c and nodes with the specified IDs.
// Duplicate IDs and IDs not found in the manager are ignored.
func (c Configuration) Add(ids ...uint32) Configuration {
	if len(c) == 0 {
		return nil
	}
	mgr := c.Manager()
	nodes := slices.Clone(c)
	// seenIDs is used to filter duplicate IDs and IDs already added
	seenIDs := newSet(c.NodeIDs()...)
	for _, id := range ids {
		if !seenIDs.contains(id) {
			if node, found := mgr.Node(id); found {
				nodes = append(nodes, node)
				seenIDs.add(id)
			}
		}
	}
	OrderedBy(ID).Sort(nodes)
	return nodes
}

// Union returns a new Configuration containing all nodes from both c and other.
// Duplicate nodes are included only once.
func (c Configuration) Union(other Configuration) Configuration {
	if len(c) == 0 {
		return slices.Clone(other)
	}
	if len(other) == 0 {
		return slices.Clone(c)
	}
	return c.Add(other.NodeIDs()...)
}

// Remove returns a new Configuration excluding nodes with the specified IDs.
func (c Configuration) Remove(ids ...uint32) Configuration {
	if len(c) == 0 {
		return nil
	}
	removeSet := newSet(ids...)
	nodes := make(Configuration, 0, len(c))
	for _, n := range c {
		if !removeSet.contains(n.id) {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// Difference returns a new Configuration with nodes from c that are not in other.
func (c Configuration) Difference(other Configuration) Configuration {
	if len(c) == 0 {
		return nil
	}
	if len(other) == 0 {
		return slices.Clone(c)
	}
	return c.Remove(other.NodeIDs()...)
}

// WithoutErrors returns a new Configuration excluding nodes that failed in the
// given QuorumCallError. If specific error types are provided, only nodes whose
// errors match one of those types (using errors.Is) will be excluded.
// If no error types are provided, all failed nodes are excluded.
func (c Configuration) WithoutErrors(err QuorumCallError, errorTypes ...error) Configuration {
	if len(c) == 0 {
		return nil
	}
	// Decide whether an error should exclude a node.
	exclude := func(cause error) bool {
		if len(errorTypes) == 0 {
			return true // no filter => exclude all failed nodes
		}
		for _, t := range errorTypes {
			if errors.Is(cause, t) {
				return true // match found
			}
		}
		return false
	}
	// Build a set of node IDs to exclude.
	excludeSet := newSet[uint32]()
	for _, ne := range err.errors {
		if exclude(ne.cause) {
			excludeSet.add(ne.nodeID)
		}
	}
	// Build configuration with remaining nodes.
	nodes := make(Configuration, 0, len(c))
	for _, node := range c {
		if !excludeSet.contains(node.id) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

type set[K comparable] map[K]struct{}

func newSet[K comparable](elems ...K) set[K] {
	set := make(set[K], len(elems))
	for _, elem := range elems {
		set.add(elem)
	}
	return set
}

func (s set[K]) add(k K) {
	s[k] = struct{}{}
}

func (s set[K]) contains(k K) bool {
	_, ok := s[k]
	return ok
}
