package gorums

import (
	"sync"
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
			replies := make(chan *gorumsStreamResult, numNodes)
			md, f := rq.newCall(methodName, replies)
			rq.putResult2(md.MessageID, result)
			f()
		}
	})
	b.Run("NewCall2", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			md, _, f := rq.newCall2(numNodes, methodName)
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

func (m *receiveQueue) newCall2(nodes int, method string) (*ordering.Metadata, chan *gorumsStreamResult, func()) {
	msgID := m.nextMsgID()
	replyChan := make(chan *gorumsStreamResult, nodes)
	m.putChan(msgID, replyChan)
	md := &ordering.Metadata{
		MessageID: msgID,
		Method:    method,
	}
	return md, replyChan, func() { m.deleteChan(msgID) }
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
