package gorums_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums"
	"github.com/relab/gorums/tests/dummy"
)

var (
	nodes   = []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}
	nodeMap = map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
)

func TestNewConfigurationEmptyNodeList(t *testing.T) {
	wantErr := errors.New("config: missing required node addresses")
	_, err := gorums.NewRawConfiguration(gorums.WithNodeList([]string{}))
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, wantErr)
	}
	if err.Error() != wantErr.Error() {
		t.Errorf("Error = %q, expected %q", err.Error(), wantErr)
	}
}

func TestNewConfigurationNodeList(t *testing.T) {
	cfg, err := gorums.NewRawConfiguration(gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodes) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodes))
	}

	contains := func(nodes []*gorums.RawNode, addr string) bool {
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

	if len(cfg.AllNodeIDs()) != len(nodes) {
		t.Errorf("mgr.Size() = %d, expected %d", len(cfg.AllNodeIDs()), len(nodes))
	}
	mgrNodes := cfg.AllNodes()
	for _, n := range nodes {
		if !contains(mgrNodes, n) {
			t.Errorf("mgr.Nodes() = %v, expected %s", mgrNodes, n)
		}
	}
}

func TestNewConfigurationNodeMap(t *testing.T) {
	cfg, err := gorums.NewRawConfiguration(gorums.WithNodeMap(nodeMap))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Size() != len(nodeMap) {
		t.Errorf("cfg.Size() = %d, expected %d", cfg.Size(), len(nodeMap))
	}
	for _, node := range cfg.Nodes() {
		if nodeMap[node.Address()] != node.ID() {
			t.Errorf("cfg.Nodes()[%s] = %d, expected %d", node.Address(), node.ID(), nodeMap[node.Address()])
		}
	}
	if len(cfg.AllNodeIDs()) != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", len(cfg.AllNodeIDs()), len(nodeMap))
	}
	for _, node := range cfg.AllNodes() {
		if nodeMap[node.Address()] != node.ID() {
			t.Errorf("mgr.Nodes()[%s] = %d, expected %d", node.Address(), node.ID(), nodeMap[node.Address()])
		}
	}
}

func TestNewConfigurationNodeIDs(t *testing.T) {
	c1, err := gorums.NewRawConfiguration(gorums.WithNodeList(nodes), gorums.WithNoConnect())
	if err != nil {
		t.Fatal(err)
	}
	if c1.Size() != len(nodes) {
		t.Errorf("c1.Size() = %d, expected %d", c1.Size(), len(nodes))
	}

	// Identical configurations c1 == c2
	nodeIDs := c1.NodeIDs()
	c2, err := c1.SubRawConfiguration(gorums.WithNodeIDs(nodeIDs))
	if err != nil {
		t.Fatal(err)
	}
	if c2.Size() != len(nodes) {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), len(nodes))
	}
	if diff := cmp.Diff(c1, c2); diff != "" {
		t.Errorf("Expected same configurations, but got (-c1 +c2):\n%s", diff)
	}

	// Configuration with one less node |c3| == |c1| - 1
	c3, err := c1.SubRawConfiguration(gorums.WithNodeIDs(nodeIDs[:len(nodeIDs)-1]))
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)-1 {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes)-1)
	}
	if diff := cmp.Diff(c1, c3); diff == "" {
		t.Errorf("Expected different configurations, but found no difference")
	}
}

func TestNewConfigurationAnd(t *testing.T) {
	c1, err := gorums.NewRawConfiguration(gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	c2Nodes := []string{"127.0.0.1:8080"}
	c2, err := c1.SubRawConfiguration(gorums.WithNodeList(c2Nodes))
	if err != nil {
		t.Fatal(err)
	}

	// Add newNodes to c1, giving a new c3 with a total of 3+2 nodes
	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := c1.SubRawConfiguration(
		c1.WithNewNodes(gorums.WithNodeList(newNodes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)+len(newNodes) {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes)+len(newNodes))
	}

	// Combine c2 to c1, giving a new c4 with a total of 3+1 nodes
	c4, err := c1.SubRawConfiguration(
		c1.And(c2),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c4.Size() != len(nodes)+len(c2Nodes) {
		t.Errorf("c4.Size() = %d, expected %d", c4.Size(), len(nodes)+len(c2Nodes))
	}

	// Combine c2 to c4, giving a new c5 with a total of 4 nodes
	// c4 already contains all nodes from c2 (see above): c4 = c1+c2
	// c5 should essentially just be a copy of c4 (ignoring duplicates from c2)
	c5, err := c1.SubRawConfiguration(
		c4.And(c2),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c5.Size() != c4.Size() {
		t.Errorf("c5.Size() = %d, expected %d", c5.Size(), c4.Size())
	}
}

func TestNewConfigurationExcept(t *testing.T) {
	c1, err := gorums.NewRawConfiguration(gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := c1.SubRawConfiguration(
		c1.WithoutNodes(c1.Nodes()[0].ID()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), c1.Size()-1)
	}

	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := c1.SubRawConfiguration(
		c1.WithNewNodes(gorums.WithNodeList(newNodes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	c4, err := c1.SubRawConfiguration(
		c3.Except(c1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c4.Size() != c3.Size()-c1.Size() {
		t.Errorf("c4.Size() = %d, expected %d", c4.Size(), c3.Size()-c1.Size())
	}
}

func TestConfigConcurrentAccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		return initServer()
	})
	defer teardown()

	cfg, err := dummy.NewConfiguration(gorums.WithNodeList(addrs), gorumsTestMgrOpts()...)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			node := cfg.Nodes()[0]
			_, err := node.Test(context.Background(), &dummy.Empty{})
			if err != nil {
				errCh <- err
			}
			wg.Done()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestConfigRelease(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 10, func(_ int) gorums.ServerIface {
		return initServer()
	})
	defer teardown()

	c1, err := dummy.NewConfiguration(gorums.WithNodeList(addrs[:3]), gorumsTestMgrOpts()...)
	if err != nil {
		t.Fatal(err)
	}

	c2, err := c1.SubConfiguration(gorums.WithNodeList(addrs[2:8]))
	if err != nil {
		t.Fatal(err)
	}

	c3, err := c1.SubConfiguration(gorums.WithNodeList(addrs[5:]))
	if err != nil {
		t.Fatal(err)
	}

	allNodes := c2.AllNodeIDs()

	c1c2, err := c2.SubConfiguration(c1.And(c2))
	if err != nil {
		t.Fatal(err)
	}
	targetNodes := c1c2.NodeIDs()

	c3.Close()

	remainingNodes := c2.AllNodeIDs()

	hasReleased := reflect.DeepEqual(remainingNodes, targetNodes)
	hasAll := reflect.DeepEqual(remainingNodes, allNodes)

	if hasAll {
		t.Fatal("c3.Close() did not close any nodes")
	}

	if !hasReleased {
		t.Fatal("There are not closed nodes which don't belong to any not closed configuration")
	}
}
