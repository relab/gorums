package gorums

import "testing"

func TestCallOptionsMustWaitSendDone(t *testing.T) {
	tests := []struct {
		name             string
		callOpts         callOptions
		wantWaitSendDone bool
	}{
		// One-way call types
		{name: "Unicast/WithSendWaiting", callOpts: getCallOptions(E_Unicast, nil), wantWaitSendDone: true},
		{name: "Unicast/WithNoSendWaiting", callOpts: getCallOptions(E_Unicast, []CallOption{WithNoSendWaiting()}), wantWaitSendDone: false},
		{name: "Multicast/WithSendWaiting", callOpts: getCallOptions(E_Multicast, nil), wantWaitSendDone: true},
		{name: "Multicast/WithNoSendWaiting", callOpts: getCallOptions(E_Multicast, []CallOption{WithNoSendWaiting()}), wantWaitSendDone: false},
		// Two-way call types (never wait for send completion, regardless of option)
		{name: "Rpc/WithSendWaiting", callOpts: getCallOptions(E_Rpc, nil), wantWaitSendDone: false},
		{name: "Rpc/WithNoSendWaiting", callOpts: getCallOptions(E_Rpc, []CallOption{WithNoSendWaiting()}), wantWaitSendDone: false},
		{name: "Quorumcall/WithSendWaiting", callOpts: getCallOptions(E_Quorumcall, nil), wantWaitSendDone: false},
		{name: "Quorumcall/WithNoSendWaiting", callOpts: getCallOptions(E_Quorumcall, []CallOption{WithNoSendWaiting()}), wantWaitSendDone: false},
		{name: "Correctable/WithSendWaiting", callOpts: getCallOptions(E_Correctable, nil), wantWaitSendDone: false},
		{name: "Correctable/WithNoSendWaiting", callOpts: getCallOptions(E_Correctable, []CallOption{WithNoSendWaiting()}), wantWaitSendDone: false},
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
