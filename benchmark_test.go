package gorums

import (
	"fmt"
	"testing"
)

func BenchmarkGetCallOptions(b *testing.B) {
	interceptor := func(ctx *clientCtx[msg, msg]) {}
	tests := []struct {
		numOpts int
	}{
		{0}, {1}, {2}, {3}, {4}, {5},
	}

	for _, tc := range tests {
		opts := make([]CallOption, tc.numOpts)
		for i := range tc.numOpts {
			opts[i] = Interceptors(interceptor)
		}
		b.Run(fmt.Sprintf("options=%d", tc.numOpts), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = getCallOptions(E_Quorumcall, opts...)
			}
		})
	}
}
