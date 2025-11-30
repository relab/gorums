package gorums

import (
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"
)

func BenchmarkGetCallOptions(b *testing.B) {
	qf := func(ctx *ClientCtx[*emptypb.Empty, *emptypb.Empty]) (*emptypb.Empty, error) {
		return nil, nil
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = getCallOptions(E_Quorumcall, WithQuorumFunc(qf))
	}
}
