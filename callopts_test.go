package gorums

import (
	"fmt"
	"testing"
)

func TestCallOptionsMustWaitSendDone(t *testing.T) {
	tests := []struct {
		name             string
		callOpts         callOptions
		wantWaitSendDone bool
	}{
		// One-way call types
		{name: "Unicast/Default", callOpts: getCallOptions(E_Unicast), wantWaitSendDone: true},
		{name: "Unicast/IgnoreErrors", callOpts: getCallOptions(E_Unicast, IgnoreErrors()), wantWaitSendDone: false},
		{name: "Multicast/Default", callOpts: getCallOptions(E_Multicast), wantWaitSendDone: true},
		{name: "Multicast/IgnoreErrors", callOpts: getCallOptions(E_Multicast, IgnoreErrors()), wantWaitSendDone: false},
		// Two-way call types (never wait for send completion, regardless of option)
		{name: "Rpc/Default", callOpts: getCallOptions(E_Rpc), wantWaitSendDone: false},
		{name: "Rpc/IgnoreErrors", callOpts: getCallOptions(E_Rpc, IgnoreErrors()), wantWaitSendDone: false},
		{name: "Quorumcall/Default", callOpts: getCallOptions(E_Quorumcall), wantWaitSendDone: false},
		{name: "Quorumcall/IgnoreErrors", callOpts: getCallOptions(E_Quorumcall, IgnoreErrors()), wantWaitSendDone: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWaitSendDone := tt.callOpts.mustWaitSendDone()
			if gotWaitSendDone != tt.wantWaitSendDone {
				t.Errorf("mustWaitSendDone() = %v, want %v", gotWaitSendDone, tt.wantWaitSendDone)
			}
		})
	}
}

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
