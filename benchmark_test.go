package gorums

import (
	"testing"
)

func BenchmarkGetCallOptions(b *testing.B) {
	interceptor := func(ctx *clientCtx[msg, msg]) {}
	b.ReportAllocs()

	for b.Loop() {
		_ = getCallOptions(E_Quorumcall, Interceptors(interceptor))
	}
}
