package conn

import (
	"math"
	"testing"

	"google.golang.org/grpc/metadata"
)

func FuzzNodeIDMetadata(f *testing.F) {
	for _, value := range []string{"", "0", "1", "4294967295", "4294967296", "-1", "+1", " 1", "001", "1\x00"} {
		f.Add(value, "2", true)
	}
	f.Add("1", "2", false)
	f.Fuzz(func(t *testing.T, first, second string, present bool) {
		if len(first)+len(second) > 4096 {
			t.Skip()
		}
		md := metadata.MD{}
		if present {
			md[gorumsNodeIDKey] = []string{first, second}
		}
		ctx := metadata.NewIncomingContext(t.Context(), md)
		// Independent decimal parser with checked accumulation. Metadata uses only
		// the first value; later claims must not override it.
		var value uint64
		valid := present && len(first) > 0
		for _, digit := range []byte(first) {
			if digit < '0' || digit > '9' {
				valid = false
				break
			}
			value = value*10 + uint64(digit-'0')
			if value > math.MaxUint32 {
				valid = false
				break
			}
		}
		var want ID
		if valid {
			want = ID(value)
		}
		if got := nodeID(ctx); got != want {
			t.Fatalf("nodeID(%q, %q) = %d, want %d", first, second, got, want)
		}
		if hasPeerMetadata(ctx) != present {
			t.Fatal("peer capability differs from metadata presence")
		}
		roundTrip := metadata.NewIncomingContext(t.Context(), MetadataWithNodeID(want))
		if nodeID(roundTrip) != want || !hasPeerMetadata(roundTrip) {
			t.Fatal("node ID metadata round trip failed")
		}
	})
}
