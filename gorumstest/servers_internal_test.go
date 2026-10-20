package gorumstest

import (
	"net"
	"strings"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/servers"
)

// closeRecorder records cleanup of a listener owned by the test harness.
type closeRecorder struct {
	net.Listener
	closed bool
}

func (lis *closeRecorder) Close() error {
	lis.closed = true
	return lis.Listener.Close()
}

type identityNode struct {
	mock.NodeAddr
	identity string
}

func (node identityNode) TLSIdentity() string { return node.identity }

// TestServersLocalNodesErrors verifies that node factory errors close every
// allocated listener and return no servers or stop function. The test uses
// the same listeners and options as LocalServers so both build modes apply.
func TestServersLocalNodesErrors(t *testing.T) {
	tests := []struct {
		name string
		node func(gorums.ID, string) gorums.NodeAddress
		want string
	}{
		{
			name: "NilNode",
			node: func(gorums.ID, string) gorums.NodeAddress { return nil },
			want: "must use listener address",
		},
		{
			name: "WrongAddress",
			node: func(gorums.ID, string) gorums.NodeAddress { return mock.NodeAddr("127.0.0.1:1") },
			want: "must use listener address",
		},
		{
			name: "EmptyIdentity",
			node: func(_ gorums.ID, addr string) gorums.NodeAddress {
				return identityNode{NodeAddr: mock.NodeAddr(addr)}
			},
			want: "node 1 has an empty TLS identity",
		},
		{
			name: "DuplicateIdentity",
			node: func(_ gorums.ID, addr string) gorums.NodeAddress {
				return identityNode{mock.NodeAddr(addr), "spiffe://relab/shared"}
			},
			want: "nodes 1 and 2 have the same TLS identity",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listen := servers.ListenFunc(t)
			var listeners []*closeRecorder
			opts := localServerOptions(t, []Option{
				gorums.WithLocalNodes(tt.node),
				gorums.WithLocalListeners(func() (net.Listener, error) {
					lis, err := listen()
					if err != nil {
						return nil, err
					}
					recorder := &closeRecorder{Listener: lis}
					listeners = append(listeners, recorder)
					return recorder, nil
				}),
			})
			srvs, stop, err := gorums.NewLocalServers(3, opts...)
			if stop != nil {
				t.Cleanup(stop)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewLocalServers error = %v; want %q", err, tt.want)
			}
			if srvs != nil || stop != nil {
				t.Errorf("NewLocalServers = %v, %v; want nil servers and stop function", srvs, stop != nil)
			}
			if len(listeners) != 3 {
				t.Fatalf("allocated %d listeners; want 3", len(listeners))
			}
			for i, lis := range listeners {
				if !lis.closed {
					t.Errorf("listener %d was not closed on failure", i)
					_ = lis.Close()
				}
			}
		})
	}
}
