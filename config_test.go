package gorums_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/encoding"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

var nodes = []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}

type testNode struct {
	addr string
}

func (n testNode) Addr() string {
	return n.addr
}

func TestNewConfigurationEmptyNodeList(t *testing.T) {
	wantErr := errors.New("config: missing required node addresses")
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	_, err := gorums.NewConfiguration(mgr, gorums.WithNodeList([]string{}))
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, wantErr)
	}
	if err.Error() != wantErr.Error() {
		t.Errorf("Error = %q, expected %q", err.Error(), wantErr)
	}
}

func TestNewConfigurationNodeList(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodes) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodes))
	}

	contains := func(nodes []*gorums.Node, addr string) bool {
		for _, node := range nodes {
			if addr == node.Address() {
				return true
			}
		}
		return false
	}
	cfgNodes := cfg.Nodes()
	for _, n := range nodes {
		if !contains(cfgNodes, n) {
			t.Errorf("cfg.Nodes() = %v, expected %s", cfgNodes, n)
		}
	}

	if mgr.Size() != len(nodes) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodes))
	}
	mgrNodes := cfg.Nodes()
	for _, n := range nodes {
		if !contains(mgrNodes, n) {
			t.Errorf("mgr.Nodes() = %v, expected %s", mgrNodes, n)
		}
	}
}

func TestNewConfigurationWithNodes(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	nodeMap := map[uint32]testNode{
		1: {addr: "127.0.0.1:9080"},
		2: {addr: "127.0.0.1:9081"},
		3: {addr: "127.0.0.1:9082"},
		4: {addr: "127.0.0.1:9083"},
	}
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodes(nodeMap))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodeMap) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodeMap))
	}
	for _, node := range cfg.Nodes() {
		if nodeMap[node.ID()].addr != node.Address() {
			t.Errorf("cfg.Nodes()[%d] = %s, expected %s", node.ID(), node.Address(), nodeMap[node.ID()].addr)
		}
	}
	if mgr.Size() != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodeMap))
	}
	for _, node := range mgr.Nodes() {
		if nodeMap[node.ID()].addr != node.Address() {
			t.Errorf("mgr.Nodes()[%d] = %s, expected %s", node.ID(), node.Address(), nodeMap[node.ID()].addr)
		}
	}
}

func TestWithNodesRejectsZeroID(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	nodeMap := map[uint32]testNode{
		0: {addr: "127.0.0.1:9080"}, // ID 0 should be rejected
		1: {addr: "127.0.0.1:9081"},
	}
	_, err := gorums.NewConfiguration(mgr, gorums.WithNodes(nodeMap))
	if err == nil {
		t.Fatal("Expected error for node ID 0, got nil")
	}
	wantErr := "config: node ID 0 is reserved; use IDs > 0"
	if err.Error() != wantErr {
		t.Errorf("Error = %q, expected %q", err.Error(), wantErr)
	}
}

func TestConfigurationExtend(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	// Create configuration with only first 2 nodes
	c, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if c.Size() != len(nodes)-1 {
		t.Errorf("c.Size() = %d, expected %d", c.Size(), len(nodes)-1)
	}
	if mgr.Size() != len(nodes)-1 {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodes)-1)
	}

	// Extend configuration with the last node
	c2, err := c.Extend(gorums.WithNodeList(nodes[2:]))
	if err != nil {
		t.Fatal(err)
	}
	if c2.Size() != len(nodes) {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), len(nodes))
	}
	if mgr.Size() != len(nodes) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodes))
	}
}

func TestConfigurationAdd(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}
	if c1.Size() != len(nodes) {
		t.Errorf("c1.Size() = %d, expected %d", c1.Size(), len(nodes))
	}

	// Add existing c1 IDs (should return same configuration)
	nodeIDs := c1.NodeIDs()
	c2 := c1.Add(nodeIDs...) // c2 = {1, 2, 3}
	if c2.Size() != len(nodes) {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), len(nodes))
	}
	if diff := cmp.Diff(c1, c2); diff != "" {
		t.Errorf("Expected same configurations, but got (-c1 +c2):\n%s", diff)
	}

	// Add non-existent ID (should be ignored)
	c3 := c1.Add(999) // c3 = {1, 2, 3}
	if c3.Size() != len(nodes) {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes))
	}
}

func TestConfigurationUnion(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	// Add newNodes to c1 using Extend (gets IDs 4, 5)
	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := c1.Extend(gorums.WithNodeList(newNodes)) // c3 = {1, 2, 3, 4, 5}
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)+len(newNodes) {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes)+len(newNodes))
	}

	// Create c2 as a subset of c1 (first two nodes)
	c2 := c1.Remove(c1[2].ID()) // c2 = {1, 2}
	if c2.Size() != 2 {
		t.Errorf("c2.Size() = %d, expected 2", c2.Size())
	}

	// Union c1 with c2 should equal c1 (since c2 is a subset)
	c4 := c1.Union(c2) // c4 = {1, 2, 3}
	if c4.Size() != c1.Size() {
		t.Errorf("c4.Size() = %d, expected %d", c4.Size(), c1.Size())
	}

	// Union c2 with c3 should include all 5 nodes
	c5 := c2.Union(c3)          // c5 = {1, 2, 3, 4, 5}
	if c5.Size() != c3.Size() { // c3 already has IDs 1,2,3,4,5
		t.Errorf("c5.Size() = %d, expected %d", c5.Size(), c3.Size())
	}
}

func TestConfigurationRemove(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	// Remove one node using Remove
	c2 := c1.Remove(c1[0].ID()) // c2 = {2, 3}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), c1.Size()-1)
	}
}

func TestConfigurationDifference(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes)) // c1 = {1, 2, 3}
	if err != nil {
		t.Fatal(err)
	}

	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := c1.Extend(gorums.WithNodeList(newNodes)) // c3 = {1, 2, 3, 4, 5}
	if err != nil {
		t.Fatal(err)
	}

	// c4 = c3 - c1 (should be just the new nodes)
	c4 := c3.Difference(c1) // c4 = {4, 5}
	if c4.Size() != c3.Size()-c1.Size() {
		t.Errorf("c4.Size() = %d, expected %d", c4.Size(), c3.Size()-c1.Size())
	}
}

func TestConfigConcurrentAccess(t *testing.T) {
	node := gorums.TestNode(t, gorums.DefaultTestServer)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := gorums.RPCCall(node.Context(t.Context()), pb.String(""), mock.TestMethod)
			if err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestConfigurationWithoutErrors(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(gorums.Closer(t, mgr))

	nodeMap := map[uint32]testNode{
		1: {addr: "127.0.0.1:9080"},
		2: {addr: "127.0.0.1:9081"},
		3: {addr: "127.0.0.1:9082"},
		4: {addr: "127.0.0.1:9083"},
	}
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodes(nodeMap))
	if err != nil {
		t.Fatal(err)
	}

	timeoutErr := errors.New("timeout")
	connRefusedErr := errors.New("connection refused")
	otherErr := errors.New("other error")
	differentErr := errors.New("different error")

	tests := []struct {
		name         string
		qcErr        gorums.QuorumCallError
		errorTypes   []error
		wantExcluded []uint32
	}{
		{
			name:         "ExcludeAllFailedNodes",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   nil,
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "ExcludeNodesWithSpecificError",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr},
			wantExcluded: []uint32{1},
		},
		{
			name:         "ExcludeNodesWithMultipleErrorTypes",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr, 3: otherErr}),
			errorTypes:   []error{timeoutErr, connRefusedErr},
			wantExcluded: []uint32{1, 2},
		},
		{
			name:         "NoMatchingErrors",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{1: timeoutErr, 2: connRefusedErr}),
			errorTypes:   []error{differentErr},
			wantExcluded: []uint32{},
		},
		{
			name:         "EmptyErrors",
			qcErr:        gorums.TestQuorumCallError(t, map[uint32]error{}),
			errorTypes:   nil,
			wantExcluded: []uint32{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCfg := cfg.WithoutErrors(tt.qcErr, tt.errorTypes...)

			// Check that excluded nodes are not in the new configuration
			newNodeIDs := newCfg.NodeIDs()
			for _, excludedID := range tt.wantExcluded {
				for _, nodeID := range newNodeIDs {
					if nodeID == excludedID {
						t.Errorf("Expected node %d to be excluded, but found it in configuration", excludedID)
					}
				}
			}

			// Check that all other nodes are still in the configuration
			expectedSize := cfg.Size() - len(tt.wantExcluded)
			if newCfg.Size() != expectedSize {
				t.Errorf("newCfg.Size() = %d, expected %d", newCfg.Size(), expectedSize)
			}
		})
	}
}
