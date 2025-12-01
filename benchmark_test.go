package gorums

import (
	"testing"
)

func BenchmarkGetCallOptions(b *testing.B) {
	interceptor := func(r *Responses[msg, msg]) {}
	b.ReportAllocs()

	for b.Loop() {
		_ = getCallOptions(E_Quorumcall, Interceptors(interceptor))
	}
}
