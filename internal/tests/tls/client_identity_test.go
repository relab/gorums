package tls

import (
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
)

// TestClientIdentityCheck verifies that a [gorums.NewConfig] client checks
// the certificate of each server whose node declares a TLS identity, and that
// it needs no client certificate to do so.
func TestClientIdentityCheck(t *testing.T) {
	ca := newTestCA(t)
	servers, _ := peerGroup(t, ca, 3, nil)
	gorumstest.WaitForPeers(t, servers)
	dialOpt := gorumstest.DialOptions(t)

	tests := []struct {
		name       string
		identities map[gorums.ID]gorums.ID // node ID -> ID of the replica whose identity it expects
		wantServe  bool
	}{
		{name: "MatchingIdentities", identities: map[gorums.ID]gorums.ID{1: 1, 2: 2, 3: 3}, wantServe: true},
		{name: "WrongIdentity", identities: map[gorums.ID]gorums.ID{1: 1, 2: 4, 3: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := make(map[gorums.ID]peerNode, len(servers))
			for _, srv := range servers {
				nodes[srv.NodeID()] = peerNode{srv.Addr(), nodeIdentity(tt.identities[srv.NodeID()])}
			}
			cfg, closeFn, err := gorums.NewConfig(gorums.WithNodes(nodes),
				dialOpt,
				gorums.WithTransportCredentials(ca.dialCreds()),
			)
			if err != nil {
				t.Fatalf("NewConfig: %v", err)
			}
			t.Cleanup(closeFn)

			// Node 1 expects its own identity in every case.
			if got, err := remoteCall(t, cfg.Node(1), 3*time.Second); err != nil || got != "server 1 from 0: hello" {
				t.Errorf("call to node 1 = %q, %v; want \"server 1 from 0: hello\", nil", got, err)
			}
			node2 := cfg.Node(2)
			if tt.wantServe {
				if got, err := remoteCall(t, node2, 3*time.Second); err != nil || got != "server 2 from 0: hello" {
					t.Errorf("call to node 2 = %q, %v; want \"server 2 from 0: hello\", nil", got, err)
				}
				return
			}
			err = awaitCode(t, node2, codes.Unauthenticated)
			if want := "gorums: node 2 failed verification"; !strings.Contains(err.Error(), want) {
				t.Errorf("call to node 2: err = %q; want it to contain %q", err, want)
			}
		})
	}
}

// TestClientIdentityInsecure verifies that a client whose nodes declare TLS
// identities refuses a server that it dials without TLS.
func TestClientIdentityInsecure(t *testing.T) {
	servers := gorumstest.LocalServers(t, 1)
	cfg, closeFn, err := gorums.NewConfig(
		gorums.WithNodes(map[gorums.ID]peerNode{1: {servers[0].Addr(), nodeIdentity(1)}}),
		gorumstest.DialOptions(t),
		gorums.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	t.Cleanup(closeFn)
	err = awaitCode(t, cfg.Node(1), codes.Unauthenticated)
	if want := "gorums: node 1 failed verification: connection does not use TLS"; !strings.Contains(err.Error(), want) {
		t.Errorf("call to node 1: err = %q; want it to contain %q", err, want)
	}
}
