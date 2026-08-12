package conn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Config represents a static set of nodes on which multicast or
// quorum calls may be invoked. A configuration is created using [NewConfig].
// A configuration should be treated as immutable. Therefore, methods that
// operate on a configuration always return a new Config instance.
type Config []*Node

// ConfigContext is a context that carries a configuration for multicast or
// quorum calls. It embeds context.Context and provides access to the configuration.
//
// Use [Config.Context] to create a ConfigContext from an existing context.
type ConfigContext struct {
	context.Context
	cfg Config
}

// Config returns the configuration associated with this context.
func (c ConfigContext) Config() Config {
	return c.cfg
}

// Context creates a new ConfigContext from the given parent context
// and this configuration. It panics if the configuration is empty.
//
// Example:
//
//	config, _ := gorums.NewConfig(gorums.WithNodeList(addrs), dialOpts...)
//	cfgCtx := config.Context(context.Background())
//	resp, err := paxos.Prepare(cfgCtx, req).Majority()
func (c Config) Context(parent context.Context) *ConfigContext {
	if len(c) == 0 {
		panic("gorums: Context called on an empty configuration")
	}
	return &ConfigContext{Context: parent, cfg: c}
}

// NewConfig returns a new [Config] based on the provided nodes and dial options.
//
// Example:
//
//	cfg, err := NewConfig(
//	    gorums.WithNodeList([]string{"localhost:8080", "localhost:8081", "localhost:8082"}),
//	    gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
//	)
func NewConfig(nodes NodeSource, opts ...DialOption) (Config, error) {
	if nodes == nil {
		return nil, fmt.Errorf("gorums: missing required node list")
	}
	mgr := newOutboundManager(opts...)
	if err := mgr.opts.Err; err != nil {
		_ = mgr.Close()
		return nil, err
	}
	cfg, err := nodes.newConfig(mgr)
	if err != nil {
		_ = mgr.Close()
		return nil, err
	}
	return cfg, nil
}

// Extend returns a new Config combining c with new nodes from the provided NodeSource.
func (c Config) Extend(nodes NodeSource) (Config, error) {
	if len(c) == 0 {
		return nil, fmt.Errorf("gorums: cannot extend empty configuration")
	}
	if nodes == nil {
		return slices.Clone(c), nil
	}
	mgr := c.mgr()
	newNodes, err := nodes.newConfig(mgr)
	if err != nil {
		return nil, err
	}
	return c.Union(newNodes), nil
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c Config) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
func (c Config) Nodes() []*Node {
	return c
}

// Size returns the number of nodes in this configuration.
func (c Config) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c Config) Equal(b Config) bool {
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

// mgr returns the outboundManager for this configuration's nodes.
func (c Config) mgr() *outboundManager {
	if len(c) == 0 {
		return nil
	}
	return c[0].mgr
}

// WaitForAllRequired reports whether the configuration must wait for its peers
// to connect before calls can proceed. That is only the case under stream
// deduplication with a server's inbound manager attached, where a higher-ID
// peer borrows a lower-ID peer's dialed stream and cannot dial it itself.
func WaitForAllRequired(c Config) bool {
	mgr := c.mgr()
	return mgr != nil && mgr.opts.StreamDedup && mgr.opts.InboundMgr != nil
}

// ValidateStreamDedup reports a configuration error if the configuration is set
// up for stream deduplication but the local node ID is missing or is not one of
// the server's peers. It returns nil when stream dedup is not in effect.
func ValidateStreamDedup(c Config) error {
	mgr := c.mgr()
	if mgr == nil {
		return nil
	}
	return mgr.validateStreamDedup()
}

// Close closes all node connections managed by this configuration.
func (c Config) Close() error {
	if mgr := c.mgr(); mgr != nil {
		return mgr.Close()
	}
	return nil
}

// Contains reports whether c contains a node with the given ID.
func (c Config) Contains(id uint32) bool {
	return slices.ContainsFunc(c, func(n *Node) bool { return n.id == id })
}

// Add returns a new Config containing nodes from c and nodes with the specified IDs.
// Duplicate IDs and IDs not found in the manager are ignored.
func (c Config) Add(ids ...uint32) Config {
	if len(c) == 0 {
		return nil
	}
	mgr := c.mgr()
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
	slices.SortFunc(nodes, ByID)
	return nodes
}

// Union returns a new Config containing all nodes from both c and other.
// Duplicate nodes are included only once.
func (c Config) Union(other Config) Config {
	if len(c) == 0 {
		return slices.Clone(other)
	}
	if len(other) == 0 {
		return slices.Clone(c)
	}
	return c.Add(other.NodeIDs()...)
}

// Remove returns a new Config excluding nodes with the specified IDs.
func (c Config) Remove(ids ...uint32) Config {
	if len(c) == 0 {
		return nil
	}
	removeSet := newSet(ids...)
	nodes := make(Config, 0, len(c))
	for _, n := range c {
		if !removeSet.contains(n.id) {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// Difference returns a new Config with nodes from c that are not in other.
func (c Config) Difference(other Config) Config {
	if len(c) == 0 {
		return nil
	}
	if len(other) == 0 {
		return slices.Clone(c)
	}
	return c.Remove(other.NodeIDs()...)
}

// Sort returns a new Config with nodes ordered by the given comparator.
// The original configuration is not modified.
//
// Use this with the built-in node comparator functions [ByID], [ByLastError],
// and [ByLatency]:
//
//	fastest := cfg.Sort(gorums.ByLatency)     // ascending by latency
//	healthy := cfg.Sort(gorums.ByLastError)   // no-error nodes first
//
// Comparators can be combined for multi-key ordering:
//
//	cfg.Sort(func(a, b *Node) int {
//	    if r := gorums.ByLastError(a, b); r != 0 {
//	        return r
//	    }
//	    return gorums.ByLatency(a, b)
//	})
//
// Sort uses a stable sort, so nodes with equal comparator values retain
// their original relative order.
//
// Note: quorum calls contact every node in the configuration regardless of
// order. Sorting only affects which nodes are selected when the result is
// sliced to a smaller subset, e.g., cfg.Sort(gorums.ByLatency)[:quorumSize].
// See the "Latency-Based Node Selection" section of the user guide for
// guidance on sub-configuration sizing and re-sort frequency.
func (c Config) Sort(cmp func(*Node, *Node) int) Config {
	if len(c) == 0 {
		return nil
	}
	sorted := slices.Clone(c)
	slices.SortStableFunc(sorted, cmp)
	return sorted
}

// Watch starts a background goroutine that calls derive(c) every interval and
// emits the result on the returned channel whenever it differs from the previous
// result. The initial result is always emitted before the first tick, so callers
// always receive a valid configuration immediately. The interval must be greater
// than zero, and derive must be non-nil; otherwise, Watch will panic.
//
// The derive function receives the full configuration c and returns any derived
// sub-configuration. Typical examples:
//
//	// Latency-based top-k subset:
//	cfg.Watch(ctx, 5*time.Second, func(c gorums.Config) gorums.Config {
//	    return c.Sort(gorums.ByLatency)[:quorumSize]
//	})
//
//	// Skip failed nodes first, then pick fastest:
//	cfg.Watch(ctx, 5*time.Second, func(c gorums.Config) gorums.Config {
//	    return c.WithoutErrors(lastErr).Sort(gorums.ByLatency)[:quorumSize]
//	})
//
// The returned channel has a buffer of 1. If the consumer is slow and has not
// yet read the previous update, the goroutine skips the emission and waits for
// the next tick to re-evaluate.
//
// The goroutine exits and the channel is closed when ctx is cancelled.
func (c Config) Watch(ctx context.Context, interval time.Duration, derive func(Config) Config) <-chan Config {
	if interval <= 0 {
		panic("gorums: Watch interval must be positive")
	}
	if derive == nil {
		panic("gorums: Watch derive function must be non-nil")
	}
	ch := make(chan Config, 1)
	go func() {
		defer close(ch)
		current := derive(c)
		ch <- current

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fresh := derive(c)
				if !fresh.Equal(current) {
					current = fresh
					select {
					case ch <- current:
					default: // consumer hasn't read yet; next tick will re-evaluate
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// WithoutErrors returns a new Config excluding nodes that failed in the
// given QuorumCallError. If specific error types are provided, only nodes whose
// errors match one of those types (using errors.Is) will be excluded.
// If no error types are provided, all failed nodes are excluded.
func (c Config) WithoutErrors(err QuorumCallError, errorTypes ...error) Config {
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
	nodes := make(Config, 0, len(c))
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
