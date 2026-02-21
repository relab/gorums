package gorums

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"

	"google.golang.org/grpc/metadata"
)

// gorumsNodeIDKey is the gRPC metadata key used by gorums clients to advertise
// their NodeID to the server.
// See doc/issue-peer-id.md for security considerations and future directions.
const gorumsNodeIDKey = "gorums-node-id"

// nodeID extracts the NodeID from the gorums-node-id metadata key in ctx.
// It returns 0 if the key is absent, empty, or not a valid uint32 greater than zero.
func nodeID(ctx context.Context) uint32 {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0
	}
	vals := md.Get(gorumsNodeIDKey)
	if len(vals) == 0 {
		return 0
	}
	id, err := strconv.ParseUint(vals[0], 10, 32)
	if err != nil || id == 0 {
		return 0
	}
	return uint32(id)
}

// InboundManager manages server-side awareness of connected peers.
// It is configured once at construction time with a fixed set of known peers
// and will be extended in later phases to track live connections and provide
// an auto-updated Configuration for server-initiated calls.
//
// InboundManager is safe for concurrent use.
type InboundManager struct {
	mu        sync.RWMutex
	peerAddrs map[uint32]string // known peer ID → normalized listening address
}

// NewInboundManager creates an InboundManager from the given option.
// Only WithNodes satisfies InboundNodeOption; passing WithNodeList is a
// compile-time error.
func NewInboundManager(opt InboundNodeOption) (*InboundManager, error) {
	im := &InboundManager{
		peerAddrs: make(map[uint32]string),
	}
	if err := opt.newServerConfig(im); err != nil {
		return nil, err
	}
	return im, nil
}

// KnownIDs returns the sorted list of NodeIDs the manager was configured with.
func (im *InboundManager) KnownIDs() []uint32 {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return slices.Sorted(maps.Keys(im.peerAddrs))
}

// isKnown reports whether the given NodeID is in the configured peer set.
func (im *InboundManager) isKnown(id uint32) bool {
	im.mu.RLock()
	defer im.mu.RUnlock()
	_, ok := im.peerAddrs[id]
	return ok
}
