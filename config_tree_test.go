package gorums_test

import (
	"fmt"
	"testing"

	"github.com/relab/gorums"
)

// TODO check if okay to use node id 0? the Node.ID() function returns 0 if node == nil.

func TestTreeConfiguration(t *testing.T) {
	const branchFactor = 2
	tests := []struct {
		nodeIDs    map[string]uint32 // all nodes in the configuration
		id         uint32            // my ID to create subconfigurations from
		wantConfig [][]uint32        // index: tree level, value: list of node IDs in the configuration
	}{
		{nodeIDs: nodeMapIDs(0, 8), id: 0, wantConfig: [][]uint32{{1}, {2, 3}, {4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 1, wantConfig: [][]uint32{{0}, {2, 3}, {4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 2, wantConfig: [][]uint32{{3}, {0, 1}, {4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 3, wantConfig: [][]uint32{{2}, {0, 1}, {4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 4, wantConfig: [][]uint32{{5}, {6, 7}, {0, 1, 2, 3}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 5, wantConfig: [][]uint32{{4}, {6, 7}, {0, 1, 2, 3}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 6, wantConfig: [][]uint32{{7}, {4, 5}, {0, 1, 2, 3}}},
		{nodeIDs: nodeMapIDs(0, 8), id: 7, wantConfig: [][]uint32{{6}, {4, 5}, {0, 1, 2, 3}}},
		//
		{nodeIDs: nodeMapIDs(0, 16), id: 0, wantConfig: [][]uint32{{1}, {2, 3}, {4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 1, wantConfig: [][]uint32{{0}, {2, 3}, {4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 2, wantConfig: [][]uint32{{3}, {0, 1}, {4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 3, wantConfig: [][]uint32{{2}, {0, 1}, {4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 4, wantConfig: [][]uint32{{5}, {6, 7}, {0, 1, 2, 3}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 5, wantConfig: [][]uint32{{4}, {6, 7}, {0, 1, 2, 3}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 6, wantConfig: [][]uint32{{7}, {4, 5}, {0, 1, 2, 3}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 7, wantConfig: [][]uint32{{6}, {4, 5}, {0, 1, 2, 3}, {8, 9, 10, 11, 12, 13, 14, 15}}},
		//
		{nodeIDs: nodeMapIDs(0, 16), id: 8, wantConfig: [][]uint32{{9}, {10, 11}, {12, 13, 14, 15}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 9, wantConfig: [][]uint32{{8}, {10, 11}, {12, 13, 14, 15}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 10, wantConfig: [][]uint32{{11}, {8, 9}, {12, 13, 14, 15}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 11, wantConfig: [][]uint32{{10}, {8, 9}, {12, 13, 14, 15}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 12, wantConfig: [][]uint32{{13}, {14, 15}, {8, 9, 10, 11}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 13, wantConfig: [][]uint32{{12}, {14, 15}, {8, 9, 10, 11}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 14, wantConfig: [][]uint32{{15}, {12, 13}, {8, 9, 10, 11}, {0, 1, 2, 3, 4, 5, 6, 7}}},
		{nodeIDs: nodeMapIDs(0, 16), id: 15, wantConfig: [][]uint32{{14}, {12, 13}, {8, 9, 10, 11}, {0, 1, 2, 3, 4, 5, 6, 7}}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d", len(test.nodeIDs)), func(t *testing.T) {
			mgr := gorums.NewRawManager(gorums.WithNoConnect())
			cfgs, err := gorums.SubConfigurations(mgr,
				gorums.WithTreeConfigurations(branchFactor, test.id, gorums.WithNodeMap(test.nodeIDs)))
			if err != nil {
				t.Fatal(err)
			}
			if len(cfgs) != log2(len(test.nodeIDs)) {
				t.Errorf("len(cfgs) = %d, expected %d", len(cfgs), 2)
			}
			for i, cfg := range cfgs {
				if len(cfg) != len(test.wantConfig[i]) {
					t.Errorf("len(cfg) = %d, expected %d", len(cfg), len(test.wantConfig[i]))
				}
				for _, id := range test.wantConfig[i] {
					if !cfg.Contains(id) {
						t.Errorf("{%v}.Contains(%d) = false, expected true", cfg.NodeIDs(), id)
					}
				}
				// fmt.Printf("c%d (%d): %v\n", i, cfg.Size(), cfg.NodeIDs())
			}
		})
	}
}

// nodeMapIDs returns a map of localhost node IDs for the given node count.
func nodeMapIDs(s, n int) map[string]uint32 {
	basePort := 9080
	nodeMapIDs := make(map[string]uint32)
	for i := s; i < s+n; i++ {
		nodeMapIDs[fmt.Sprintf("127.0.0.1:%d", basePort+i)] = uint32(i)
	}
	return nodeMapIDs
}

// log2 returns the base 2 logarithm of x.
func log2(x int) int {
	y := 0
	for x > 1 {
		x >>= 1
		y++
	}
	return y
}
