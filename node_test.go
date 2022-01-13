package gorums

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

type dummyNode struct {
	*RawNode
}

func (n dummyNode) AsRaw() *RawNode {
	return n.RawNode
}

func TestNodeSort(t *testing.T) {
	nodes := []dummyNode{
		{RawNode: &RawNode{
			id: 100,
			channel: &channel{
				lastError: nil,
				latency:   time.Second,
			},
		}},
		{RawNode: &RawNode{
			id: 101,
			channel: &channel{
				lastError: errors.New("some error"),
				latency:   250 * time.Millisecond,
			},
		}},
		{RawNode: &RawNode{
			id: 42,
			channel: &channel{
				lastError: nil,
				latency:   300 * time.Millisecond,
			},
		}},
		{RawNode: &RawNode{
			id: 99,
			channel: &channel{
				lastError: errors.New("some error"),
				latency:   500 * time.Millisecond,
			},
		}},
	}

	n := len(nodes)

	OrderedBy(ID[dummyNode]).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].id < nodes[i-1].id {
			t.Error("by id: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(LastNodeError[dummyNode]).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].LastErr() == nil && nodes[i-1].LastErr() != nil {
			t.Error("by error: not sorted")
			printNodes(t, nodes)
		}
	}
}

func printNodes(t *testing.T, nodes []dummyNode) {
	t.Helper()
	for i, n := range nodes {
		nodeStr := fmt.Sprintf(
			"%d: node %d | addr: %s | latency: %v | err: %v",
			i, n.id, n.addr, n.Latency(), n.LastErr())
		t.Logf("%s", nodeStr)
	}
}
