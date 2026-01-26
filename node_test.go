package gorums

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNodeSort(t *testing.T) {
	nodes := []*Node[uint32]{
		{
			id: 100,
			channel: &channel[uint32]{
				lastError: nil,
				latency:   time.Second,
			},
		},
		{
			id: 101,
			channel: &channel[uint32]{
				lastError: errors.New("some error"),
				latency:   250 * time.Millisecond,
			},
		},
		{
			id: 42,
			channel: &channel[uint32]{
				lastError: nil,
				latency:   300 * time.Millisecond,
			},
		},
		{
			id: 99,
			channel: &channel[uint32]{
				lastError: errors.New("some error"),
				latency:   500 * time.Millisecond,
			},
		},
	}

	n := len(nodes)

	OrderedBy(ID[uint32]).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].id < nodes[i-1].id {
			t.Error("by id: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(LastNodeError[uint32]).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].LastErr() == nil && nodes[i-1].LastErr() != nil {
			t.Error("by error: not sorted")
			printNodes(t, nodes)
		}
	}
}

func printNodes(t *testing.T, nodes []*Node[uint32]) {
	t.Helper()
	for i, n := range nodes {
		nodeStr := fmt.Sprintf(
			"%d: node %d | addr: %s | latency: %v | err: %v",
			i, n.id, n.addr, n.Latency(), n.LastErr())
		t.Logf("%s", nodeStr)
	}
}
