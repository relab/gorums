package broadcast

import (
	"fmt"
	"testing"
)

func TestBroadcastID(t *testing.T) {
	if maxMachineID != 4096 {
		t.Errorf("maxMachineID is hardcoded in test. want: %v, got: %v", 4096, maxMachineID)
	}
	if maxSequenceNum != 262144 {
		t.Errorf("maxSequenceNum is hardcoded in test. want: %v, got: %v", 262144, maxSequenceNum)
	}
	if maxShard != 16 {
		t.Errorf("maxShard is hardcoded in test. want: %v, got: %v", 16, maxShard)
	}
	// intentionally provide an illegal machineID. A random machineID should be given instead.
	snowflake := NewSnowflake(8000)
	machineID := snowflake.MachineID
	timestampDistribution := make(map[uint32]int)
	maxN := 262144 // = 2^18
	for j := 1; j < 3*maxN; j++ {
		i := j % maxN
		broadcastID := snowflake.NewBroadcastID()
		timestamp, shard, m, n := DecodeBroadcastID(broadcastID)
		if i != int(n) {
			t.Errorf("wrong sequence number. want: %v, got: %v", i, n)
		}
		if m >= 4096 {
			t.Errorf("machine ID cannot be higher than max. want: %v, got: %v", 4095, m)
		}
		if m != uint16(machineID) {
			t.Errorf("wrong machine ID. want: %v, got: %v", machineID, m)
		}
		if shard >= 16 {
			t.Errorf("cannot have higher shard than max. want: %v, got: %v", 15, shard)
		}
		if n >= uint32(maxN) {
			t.Errorf("sequence number cannot be higher than max. want: %v, got: %v", maxN, n)
		}
		timestampDistribution[timestamp]++
	}
	for k, v := range timestampDistribution {
		if v > maxN {
			t.Errorf("cannot have more than maxN in a second. want: %v, got: %v", maxN, k)
		}
	}
}

func TestNewSnowflake(t *testing.T) {
	const random = 0
	tests := []struct {
		machineID uint64
		wantID    uint64
	}{
		{0, random},    // should generate a random machine ID
		{4097, random}, // should generate a random machine ID
		{1, 1},         // use expected machine ID
		{4096, 4096},   // use expected machine ID
		{1234, 1234},   // use expected machine ID
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("MachineID=%d", tt.machineID), func(t *testing.T) {
			snowflake := NewSnowflake(tt.machineID)
			if tt.wantID == random {
				if snowflake.MachineID == 0 || snowflake.MachineID > 4096 {
					t.Errorf("NewSnowflake(%d) = %d, want random machine ID in range [1, 4096]", tt.machineID, snowflake.MachineID)
				}
				return
			}
			if snowflake.MachineID != tt.wantID {
				t.Errorf("NewSnowflake should use the provided machine ID. got: %v", snowflake.MachineID)
			}
		})
	}
}
