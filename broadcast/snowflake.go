package broadcast

import (
	"math/rand"
	"sync"
	"time"
)

type Snowflake struct {
	mut         sync.Mutex
	MachineID   uint64
	SequenceNum uint64
	lastT       uint64
	lastS       uint64
	epoch       time.Time
}

const (
	MaxMachineID       = uint16(1 << 12)
	maxShard           = uint8(1 << 4)
	maxSequenceNum     = uint32(1 << 18)
	bitMaskTimestamp   = uint64((1<<30)-1) << 34
	bitMaskShardID     = uint64((1<<4)-1) << 30
	bitMaskMachineID   = uint64((1<<12)-1) << 18
	bitMaskSequenceNum = uint64((1 << 18) - 1)
	epoch              = "2024-01-01T00:00:00"
)

func Epoch() time.Time {
	timestamp, _ := time.Parse("2006-01-02T15:04:05", epoch)
	return timestamp
}

func NewSnowflake(id uint64) *Snowflake {
	if id < 0 || id >= uint64(MaxMachineID) {
		id = uint64(rand.Int31n(int32(MaxMachineID)))
	}
	return &Snowflake{
		MachineID:   id,
		SequenceNum: 0,
		epoch:       Epoch(),
	}
}

func (s *Snowflake) NewBroadcastID() uint64 {
	// timestamp: 30 bit -> seconds since 01.01.2024
	// shardID: 4 bit -> 16 different shards
	// machineID: 12 bit -> 4096 clients
	// sequenceNum: 18 bit -> 262 144 messages
start:
	s.mut.Lock()
	timestamp := uint64(time.Since(s.epoch).Seconds())
	l := (s.SequenceNum + 1) % uint64(maxSequenceNum)
	if timestamp-s.lastT <= 0 && l == s.lastS {
		s.mut.Unlock()
		time.Sleep(10 * time.Millisecond)
		goto start
	}
	if timestamp > s.lastT {
		s.lastT = timestamp
		s.lastS = l
	}
	s.SequenceNum = l
	s.mut.Unlock()

	t := (timestamp << 34) & bitMaskTimestamp
	shard := (uint64(rand.Int31n(int32(maxShard))) << 30) & bitMaskShardID
	m := uint64(s.MachineID<<18) & bitMaskMachineID
	n := l & bitMaskSequenceNum
	return t | shard | m | n
}

func DecodeBroadcastID(broadcastID uint64) (timestamp uint32, shardID uint16, machineID uint16, sequenceNo uint32) {
	t := (broadcastID & bitMaskTimestamp) >> 34
	shard := (broadcastID & bitMaskShardID) >> 30
	m := (broadcastID & bitMaskMachineID) >> 18
	n := (broadcastID & bitMaskSequenceNum)
	return uint32(t), uint16(shard), uint16(m), uint32(n)
}
