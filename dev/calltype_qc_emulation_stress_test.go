package dev_test

import (
	"sync"
	"testing"
	"time"
)

// This test case tries to emulate a sequence synchornous quorum calls
// whose individual RPCs are concurrent. The objective is to show that
// with the current setup, it is possible that the server receives these
// individual RPCs in the incorrect order. It is difficult to reproduce
// this problem, and so the test must be run using the stress test command,
// as follows:
//
// % go test -c
// % stress -p=4 ./dev.test -test.run=TestQuorumCallsWaitMajority -test.cpu=50
//
// The exact parameters to the stress command can probably be changed.

type Reply struct {
	node int
	msg  int
}

var (
	serverState [][]int
	mutex       = &sync.Mutex{}
)

func TestQuorumCallsWaitMajority(t *testing.T) {
	msgs := []int{1, 2}
	nodes := []int{0, 1, 2}
	serverState = make([][]int, len(nodes))
	for _, msg := range msgs {
		QCWaitMajority(nodes, msg)
	}
	checkServerState(t, nodes, msgs)
}

func QCWaitMajority(nodes []int, msg int) {
	replyChan := make(chan Reply, len(nodes))
	for _, n := range nodes {
		go callQC(n, msg, replyChan)
	}
	majority := len(nodes)/2 + 1
	for len(replyChan) >= majority {
		return
	}
}

func callQC(node int, msg int, replies chan Reply) {
	// Emulate the IO time it takes for a RPC call to finish
	time.Sleep(1 * time.Millisecond)
	mutex.Lock()
	serverState[node] = append(serverState[node], msg)
	mutex.Unlock()
	replies <- Reply{node, msg}
}

func checkServerState(t *testing.T, nodes []int, msgs []int) {
	mutex.Lock()
	for _, node := range nodes {
		for i, m := range serverState[node] {
			if m != msgs[i] {
				t.Errorf("got msg order %v on node %d, expected %v", serverState[node], node, msgs)
			}
		}
	}
	mutex.Unlock()
}
