package dev

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNodeSort(t *testing.T) {
	nodes := []*Node{
		{
			id:      100,
			lastErr: nil,
			latency: time.Second,
		},
		{
			id:      101,
			lastErr: errors.New("some error"),
			latency: 250 * time.Millisecond,
		},
		{
			id:      42,
			lastErr: nil,
			latency: 300 * time.Millisecond,
		},
		{
			id:      99,
			lastErr: errors.New("some error"),
			latency: 500 * time.Millisecond,
		},
	}

	n := len(nodes)

	OrderedBy(Latency).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].latency < nodes[i-1].latency {
			t.Error("by latency: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(ID).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].id < nodes[i-1].id {
			t.Error("by id: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(Error).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].lastErr == nil && nodes[i-1].lastErr != nil {
			t.Error("by error: not sorted")
			printNodes(t, nodes)
		}
	}
}

func printNodes(t *testing.T, nodes []*Node) {
	for i, n := range nodes {
		nstr := fmt.Sprintf(
			"%d: node %d | addr: %s | latency: %v | err: %v",
			i, n.id, n.addr, n.latency, n.lastErr)
		t.Logf("%s", nstr)
	}
}
