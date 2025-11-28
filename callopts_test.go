package gorums

import "testing"

func TestCallOptionsMustWaitSendDone(t *testing.T) {
	tests := []struct {
		name             string
		callOpts         callOptions
		wantWaitSendDone bool
	}{
		// One-way call types
		{name: "Unicast/WithSendWaiting", callOpts: getCallOptions(E_Unicast), wantWaitSendDone: true},
		{name: "Unicast/WithNoSendWaiting", callOpts: getCallOptions(E_Unicast, WithNoSendWaiting()), wantWaitSendDone: false},
		{name: "Multicast/WithSendWaiting", callOpts: getCallOptions(E_Multicast), wantWaitSendDone: true},
		{name: "Multicast/WithNoSendWaiting", callOpts: getCallOptions(E_Multicast, WithNoSendWaiting()), wantWaitSendDone: false},
		// Two-way call types (never wait for send completion, regardless of option)
		{name: "Rpc/WithSendWaiting", callOpts: getCallOptions(E_Rpc), wantWaitSendDone: false},
		{name: "Rpc/WithNoSendWaiting", callOpts: getCallOptions(E_Rpc, WithNoSendWaiting()), wantWaitSendDone: false},
		{name: "Quorumcall/WithSendWaiting", callOpts: getCallOptions(E_Quorumcall), wantWaitSendDone: false},
		{name: "Quorumcall/WithNoSendWaiting", callOpts: getCallOptions(E_Quorumcall, WithNoSendWaiting()), wantWaitSendDone: false},
		{name: "Correctable/WithSendWaiting", callOpts: getCallOptions(E_Correctable), wantWaitSendDone: false},
		{name: "Correctable/WithNoSendWaiting", callOpts: getCallOptions(E_Correctable, WithNoSendWaiting()), wantWaitSendDone: false},
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
