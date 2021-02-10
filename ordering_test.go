package gorums

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/relab/gorums/ordering"
)

// BenchmarkReceiveQueue is here to benchmark whether or not the receiveQueue
// should be implement using RWMutex or sync.Map.
// On my machine, it is pretty clear that the RWMutex implementation
// is the better option:
// BenchmarkReceiveQueue/RWMutexMap-12         	13286848	        77.7 ns/op
// BenchmarkReceiveQueue/syncMapStruct-12      	 2467936	       482 ns/op
// BenchmarkReceiveQueue/syncMapDirect-12      	 2494368	       485 ns/op
//
// Below are results from experiments with a new NewCall method that implements
// the behavior of all uses of the receiveQueue's methods.
//
// On MacPro (Late 2013) 2,7 GHz 12-Core Intel Xeon E5:
// % go test -v -benchmem -run none -bench BenchmarkReceiveQueue
// BenchmarkReceiveQueue/NewCall-24         	 2943565	       405 ns/op	     240 B/op	       4 allocs/op
// BenchmarkReceiveQueue/RWMutexMap-24      	 4028078	       298 ns/op	     128 B/op	       2 allocs/op
// BenchmarkReceiveQueue/syncMapStruct-24   	 1281080	       929 ns/op	     464 B/op	      10 allocs/op
// BenchmarkReceiveQueue/syncMapDirect-24   	 1293362	       929 ns/op	     464 B/op	      10 allocs/op
//
// After splitting newCall into newCall and newReply, we get these results:
// % go test -v -benchmem -run none -bench BenchmarkReceiveQueue
// BenchmarkReceiveQueue/NewCall-24         	 3108373	       372 ns/op	     240 B/op	       4 allocs/op
// BenchmarkReceiveQueue/RWMutexMap-24      	 4714849	       254 ns/op	     128 B/op	       2 allocs/op
// BenchmarkReceiveQueue/syncMapStruct-24   	 1326286	       909 ns/op	     464 B/op	      10 allocs/op
// BenchmarkReceiveQueue/syncMapDirect-24   	 1326224	       909 ns/op	     464 B/op	      10 allocs/op
//
// Note that NewCall is expected to require more allocations than RWMutexMap
// because it allocates Metadata and the delete function, and will be a bit slower
// for these reasons. However, when used in QuorumCall, which also has to allocate
// Metadata, it appears to have insignificant performance overhead.
//
// Before replacing the receiveQueue methods with NewCall:
//
// cmd/benchmark/benchmark -config-size 9 -quorum-size 5 -time 5s -benchmarks ^QuorumCall
// Benchmark     Throughput         Latency    Std.dev    Client+Servers
// QuorumCall    1044.15 ops/sec    0.96 ms    0.36 ms    19912 B/op        535 allocs/op
// QuorumCall    1032.27 ops/sec    0.97 ms    0.36 ms    19901 B/op        535 allocs/op
// QuorumCall    1054.69 ops/sec    0.95 ms    0.35 ms    19921 B/op        535 allocs/op
// QuorumCall    1043.53 ops/sec    0.96 ms    0.35 ms    19930 B/op        535 allocs/op
// QuorumCall    1041.38 ops/sec    0.96 ms    0.35 ms    19893 B/op        535 allocs/op
//
// After replacing the receiveQueue methods with NewCall:
//
// cmd/benchmark/benchmark -config-size 9 -quorum-size 5 -time 5s -benchmarks ^QuorumCall
// Benchmark     Throughput         Latency    Std.dev    Client+Servers
// QuorumCall    1044.11 ops/sec    0.96 ms    0.36 ms    19928 B/op        536 allocs/op
// QuorumCall    1118.45 ops/sec    0.89 ms    0.34 ms    19953 B/op        537 allocs/op
// QuorumCall    1038.68 ops/sec    0.96 ms    0.36 ms    19914 B/op        535 allocs/op
// QuorumCall    1038.28 ops/sec    0.96 ms    0.36 ms    19954 B/op        536 allocs/op
// QuorumCall    1046.57 ops/sec    0.95 ms    0.39 ms    19955 B/op        536 allocs/op
//
// After replacing the single newCall with newCall and newReply:
//
// cmd/benchmark/benchmark -config-size 9 -quorum-size 5 -time 5s -benchmarks ^QuorumCall
// Benchmark     Throughput         Latency    Std.dev    Client+Servers
// QuorumCall    1033.38 ops/sec    0.97 ms    0.37 ms    19965 B/op        536 allocs/op
// QuorumCall    1026.32 ops/sec    0.97 ms    0.38 ms    19919 B/op        535 allocs/op
// QuorumCall    1037.70 ops/sec    0.96 ms    0.37 ms    19916 B/op        535 allocs/op
// QuorumCall    1020.35 ops/sec    0.98 ms    0.38 ms    19927 B/op        535 allocs/op
// QuorumCall    1028.64 ops/sec    0.97 ms    0.37 ms    19928 B/op        535 allocs/op
//
func BenchmarkReceiveQueue(b *testing.B) {
	const (
		numNodes   = 3
		methodName = "Some/Dummy/Method"
	)
	rq := newReceiveQueue()
	// dummy result
	result := &gorumsStreamResult{nid: 2, reply: nil, err: nil}
	b.Run("NewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			md := rq.newCall(methodName)
			_, f := rq.newReply(md, numNodes)
			rq.putResult2(md.MessageID, result)
			f()
		}
	})
	b.Run("OldNewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			md, _, f := rq.oldNewCall(methodName, numNodes, true)
			rq.putResult2(md.MessageID, result)
			f()
		}
	})
	b.Run("RWMutexMap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			replies := make(chan *gorumsStreamResult, numNodes)
			rq.putChan(msgID, replies)
			rq.putResult2(msgID, result)
			rq.deleteChan(msgID)
		}
	})
	srq := &rQueue{}
	b.Run("syncMapStruct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			replies := make(chan *gorumsStreamResult, numNodes)
			srq.putChan(msgID, replies)
			srq.putResult(msgID, result)
			srq.deleteChan(msgID)
		}
	})
	syncrq := sync.Map{}
	b.Run("syncMapDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			replies := make(chan *gorumsStreamResult, numNodes)
			syncrq.Store(msgID, replies)
			xc, ok := syncrq.Load(msgID)
			c := xc.(chan *gorumsStreamResult)
			if ok {
				// ignore sending result on channel
				_ = c
			}
			syncrq.Delete(msgID)
		}
	})
}

// newCall returns metadata for the call, a channel for receiving replies
// and a done function to be called for clean up.
// Only if reply is true will replyChan and the done function be allocated.
func (m *receiveQueue) oldNewCall(method string, maxReplies int, reply bool) (md *ordering.Metadata, replyChan chan *gorumsStreamResult, done func()) {
	msgID := atomic.AddUint64(&m.msgID, 1)
	md = &ordering.Metadata{
		MessageID: msgID,
		Method:    method,
	}
	if reply {
		replyChan = make(chan *gorumsStreamResult, maxReplies)
		m.recvQMut.Lock()
		m.recvQ[msgID] = replyChan
		m.recvQMut.Unlock()
		done = func() {
			m.recvQMut.Lock()
			delete(m.recvQ, msgID)
			m.recvQMut.Unlock()
		}
	}
	return
}

// nextMsgID is only used except for benchmarking.
func (m *receiveQueue) nextMsgID() uint64 {
	return atomic.AddUint64(&m.msgID, 1)
}

// putChan is only used except for benchmarking.
func (m *receiveQueue) putChan(id uint64, c chan *gorumsStreamResult) {
	m.recvQMut.Lock()
	m.recvQ[id] = c
	m.recvQMut.Unlock()
}

// deleteChan is only used except for benchmarking.
func (m *receiveQueue) deleteChan(id uint64) {
	m.recvQMut.Lock()
	delete(m.recvQ, id)
	m.recvQMut.Unlock()
}

func (m *receiveQueue) putResult2(id uint64, result *gorumsStreamResult) {
	m.recvQMut.RLock()
	c, ok := m.recvQ[id]
	m.recvQMut.RUnlock()
	if ok {
		// ignore sending result on channel
		_ = c
	}
}

type rQueue struct {
	sync.Map
}

func (m *rQueue) putChan(id uint64, c chan *gorumsStreamResult) {
	m.Store(id, c)
}

func (m *rQueue) deleteChan(id uint64) {
	m.Delete(id)
}

func (m *rQueue) putResult(id uint64, result *gorumsStreamResult) {
	xc, ok := m.Load(id)
	c := xc.(chan *gorumsStreamResult)
	if ok {
		// ignore sending result on channel
		_ = c
	}
}
