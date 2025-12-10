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

var (
	nodes   = []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}
	nodeMap = map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
)

func TestNewConfigurationEmptyNodeList(t *testing.T) {
	wantErr := errors.New("config: missing required node addresses")
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

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
	t.Cleanup(mgr.Close)

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

func TestNewConfigurationNodeMap(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeMap(nodeMap))
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
	if mgr.Size() != len(nodeMap) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(nodeMap))
	}
	for _, node := range mgr.Nodes() {
		if nodeMap[node.Address()] != node.ID() {
			t.Errorf("mgr.Nodes()[%s] = %d, expected %d", node.Address(), node.ID(), nodeMap[node.Address()])
		}
	}
}

func TestNewConfigurationNodeIDs(t *testing.T) {
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	if c1.Size() != len(nodes) {
		t.Errorf("c1.Size() = %d, expected %d", c1.Size(), len(nodes))
	}

	// Identical configurations c1 == c2
	nodeIDs := c1.NodeIDs()
	c2, err := gorums.NewConfiguration(mgr, gorums.WithNodeIDs(nodeIDs))
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
	c3, err := gorums.NewConfiguration(mgr, gorums.WithNodeIDs(nodeIDs[:len(nodeIDs)-1]))
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
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	c2Nodes := []string{"127.0.0.1:8080"}
	c2, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(c2Nodes))
	if err != nil {
		t.Fatal(err)
	}

	// Add newNodes to c1, giving a new c3 with a total of 3+2 nodes
	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := gorums.NewConfiguration(
		mgr,
		c1.WithNewNodes(gorums.WithNodeList(newNodes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c3.Size() != len(nodes)+len(newNodes) {
		t.Errorf("c3.Size() = %d, expected %d", c3.Size(), len(nodes)+len(newNodes))
	}

	// Combine c2 to c1, giving a new c4 with a total of 3+1 nodes
	c4, err := gorums.NewConfiguration(
		mgr,
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
	c5, err := gorums.NewConfiguration(
		mgr,
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
	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	c1, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(nodes))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := gorums.NewConfiguration(
		mgr,
		c1.WithoutNodes(c1[0].ID()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Size() != c1.Size()-1 {
		t.Errorf("c2.Size() = %d, expected %d", c2.Size(), c1.Size()-1)
	}

	newNodes := []string{"127.0.0.1:9083", "127.0.0.1:9084"}
	c3, err := gorums.NewConfiguration(
		mgr,
		c1.WithNewNodes(gorums.WithNodeList(newNodes)),
	)
	if err != nil {
		t.Fatal(err)
	}
	c4, err := gorums.NewConfiguration(
		mgr,
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
	node := gorums.TestNode(t, nil)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			nodeCtx := gorums.WithNodeContext(t.Context(), node)
			_, err := gorums.RPCCall(nodeCtx, pb.String(""), mock.TestMethod)
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
