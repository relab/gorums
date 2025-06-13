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
	timestampBits      = 30                 // seconds since 01.01.2025
	shardIDBits        = 4                  // 16 different shards
	machineIDBits      = 12                 // 4096 clients
	sequenceNumBits    = 18                 // 262 144 messages
	timestampBitsShift = 64 - timestampBits // 34

	maxShard       = uint8(1 << shardIDBits)
	maxMachineID   = uint16(1 << machineIDBits)
	maxSequenceNum = uint32(1 << sequenceNumBits)

	bitMaskTimestamp   = uint64((1<<timestampBits)-1) << timestampBitsShift
	bitMaskShardID     = uint64((1<<shardIDBits)-1) << timestampBits
	bitMaskMachineID   = uint64((1<<machineIDBits)-1) << sequenceNumBits
	bitMaskSequenceNum = uint64((1<<sequenceNumBits)-1) << 0

	epoch = "2025-01-01T00:00:00"
)

func Epoch() time.Time {
	timestamp, _ := time.Parse("2006-01-02T15:04:05", epoch)
	return timestamp
}

// NewSnowflake creates a new Snowflake instance with the given machine ID.
// If the machine ID is 0 or greater than 4096, a random machine ID will be
// generated within the range [1, 4096].
func NewSnowflake(id uint64) *Snowflake {
	if id == 0 || id > uint64(maxMachineID) {
		id = uint64(rand.Int31n(int32(maxMachineID))) + 1 // avoid 0 as the machine ID
	}
	return &Snowflake{
		MachineID:   id,
		SequenceNum: 0,
		epoch:       Epoch(),
	}
}

func (s *Snowflake) NewBroadcastID() uint64 {
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

	t := (timestamp << timestampBitsShift) & bitMaskTimestamp
	shard := (uint64(rand.Int31n(int32(maxShard))) << timestampBits) & bitMaskShardID
	m := uint64(s.MachineID<<sequenceNumBits) & bitMaskMachineID
	n := l & bitMaskSequenceNum
	return t | shard | m | n
}

func DecodeBroadcastID(broadcastID uint64) (timestamp uint32, shardID uint16, machineID uint16, sequenceNo uint32) {
	t := (broadcastID & bitMaskTimestamp) >> timestampBitsShift
	shard := (broadcastID & bitMaskShardID) >> timestampBits
	m := (broadcastID & bitMaskMachineID) >> sequenceNumBits
	n := (broadcastID & bitMaskSequenceNum)
	return uint32(t), uint16(shard), uint16(m), uint32(n)
}
