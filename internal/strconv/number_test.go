package strconv

import (
	"math"
	"testing"
)

func TestNumberParseInteger(t *testing.T) {
	t.Run("Uint32", func(t *testing.T) {
		tests := []struct {
			in      string
			want    uint32
			wantErr bool
		}{
			{in: "0", want: 0},
			{in: "4294967295", want: math.MaxUint32},
			{in: "4294967296", wantErr: true},
			{in: "-1", wantErr: true},
			{in: "x", wantErr: true},
		}
		for _, tt := range tests {
			got, err := ParseInteger[uint32](tt.in, 10)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInteger[uint32](%q) error = %v, wantErr %t", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseInteger[uint32](%q) = %d, want %d", tt.in, got, tt.want)
			}
		}
	})
	t.Run("Int8", func(t *testing.T) {
		tests := []struct {
			in      string
			want    int8
			wantErr bool
		}{
			{in: "-128", want: math.MinInt8},
			{in: "127", want: math.MaxInt8},
			{in: "128", wantErr: true},
			{in: "-129", wantErr: true},
		}
		for _, tt := range tests {
			got, err := ParseInteger[int8](tt.in, 10)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInteger[int8](%q) error = %v, wantErr %t", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseInteger[int8](%q) = %d, want %d", tt.in, got, tt.want)
			}
		}
	})
	t.Run("Base16", func(t *testing.T) {
		got, err := ParseInteger[uint16]("ff", 16)
		if err != nil || got != 255 {
			t.Errorf(`ParseInteger[uint16]("ff", 16) = %d, %v, want 255, nil`, got, err)
		}
	})
}

func TestNumberFormat(t *testing.T) {
	if got := Format(int16(-42), 10); got != "-42" {
		t.Errorf("Format(int16(-42), 10) = %q, want %q", got, "-42")
	}
	if got := Format(uint64(math.MaxUint64), 16); got != "ffffffffffffffff" {
		t.Errorf("Format(uint64(MaxUint64), 16) = %q, want %q", got, "ffffffffffffffff")
	}
	type nodeID uint32
	if got := Format(nodeID(7), 2); got != "111" {
		t.Errorf("Format(nodeID(7), 2) = %q, want %q", got, "111")
	}
}
