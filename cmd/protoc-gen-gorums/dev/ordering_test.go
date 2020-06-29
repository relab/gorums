package dev

import (
	"sync"
	"testing"
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
	rq := newReceiveQueue()
	// dummy channel
	replies := make(chan *orderingResult, 1)
	// dummy result
	result := &orderingResult{nid: 2, reply: nil, err: nil}
	b.Run("RWMutexMap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			rq.putChan(msgID, replies)
			rq.putResult2(msgID, result)
			rq.deleteChan(msgID)
		}
	})
	srq := &rQueue{}
	b.Run("syncMapStruct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			srq.putChan(msgID, replies)
			srq.putResult(msgID, result)
			srq.deleteChan(msgID)
		}
	})
	syncrq := sync.Map{}
	b.Run("syncMapDirect", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msgID := rq.nextMsgID()
			syncrq.Store(msgID, replies)
			xc, ok := syncrq.Load(msgID)
			c := xc.(chan *orderingResult)
			if ok {
				// ignore sending result on channel
				_ = c
			}
			syncrq.Delete(msgID)
		}
	})
}

func (m *receiveQueue) putResult2(id uint64, result *orderingResult) {
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

func (m *rQueue) putChan(id uint64, c chan *orderingResult) {
	m.Store(id, c)
}

func (m *rQueue) deleteChan(id uint64) {
	m.Delete(id)
}

func (m *rQueue) putResult(id uint64, result *orderingResult) {
	xc, ok := m.Load(id)
	c := xc.(chan *orderingResult)
	if ok {
		// ignore sending result on channel
		_ = c
	}
}
