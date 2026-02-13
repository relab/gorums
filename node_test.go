package gorums

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
)

func TestNodeSort(t *testing.T) {
	nodes := []*Node{
		{
			id: 100,
			channel: stream.NewChannelWithState(
				time.Second,
				nil,
			),
		},
		{
			id: 101,
			channel: stream.NewChannelWithState(
				250*time.Millisecond,
				errors.New("some error"),
			),
		},
		{
			id: 42,
			channel: stream.NewChannelWithState(
				300*time.Millisecond,
				nil,
			),
		},
		{
			id: 99,
			channel: stream.NewChannelWithState(
				500*time.Millisecond,
				errors.New("some error"),
			),
		},
	}

	n := len(nodes)

	OrderedBy(ID).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].id < nodes[i-1].id {
			t.Error("by id: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(LastNodeError).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].LastErr() == nil && nodes[i-1].LastErr() != nil {
			t.Error("by error: not sorted")
			printNodes(t, nodes)
		}
	}
}

func printNodes(t *testing.T, nodes []*Node) {
	t.Helper()
	for i, n := range nodes {
		nodeStr := fmt.Sprintf(
			"%d: node %d | addr: %s | latency: %v | err: %v",
			i, n.id, n.addr, n.Latency(), n.LastErr())
		t.Logf("%s", nodeStr)
	}
}
