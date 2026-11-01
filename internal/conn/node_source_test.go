package conn

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/credentials/insecure"
)

// insecureDial sets the transport credentials that nodes with TLS identities
// require. They are insecure, since these tests never connect.
func insecureDial(o *DialOptions) {
	o.Credentials = insecure.NewCredentials()
}

// TestNodeSourceHasIdentity verifies that any declared identity enables checks,
// including empty identities that must fail validation rather than disable it.
func TestNodeSourceHasIdentity(t *testing.T) {
	tests := []struct {
		name string
		nm   nodeMap
		want bool
	}{
		{name: "Nil"},
		{name: "Empty", nm: nodeMap{}},
		{name: "AddressOnly", nm: nodeMap{1: {addr: "127.0.0.1:9001"}}},
		{
			name: "Identity",
			nm:   nodeMap{1: {identity: "spiffe://relab/node/1", hasIdentity: true}},
			want: true,
		},
		{
			name: "EmptyIdentity",
			nm:   nodeMap{1: {hasIdentity: true}},
			want: true,
		},
		{
			name: "Mixed",
			nm: nodeMap{
				1: {addr: "127.0.0.1:9001"},
				2: {identity: "spiffe://relab/node/2", hasIdentity: true},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nm.hasIdentity(); got != tt.want {
				t.Errorf("hasIdentity() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestNodeSourceInvalidIdentity verifies that [NewConfig] and [Config.Extend]
// reject an identity-bearing source whose identities cannot all be checked,
// instead of ignoring them.
func TestNodeSourceInvalidIdentity(t *testing.T) {
	tests := []struct {
		name  string
		nodes NodeSource
		want  string
	}{
		{
			name: "Partial",
			nodes: WithNodes(map[ID]tlsNode{
				1: {"127.0.0.1:9001", "node1.relab.example"},
				2: {"127.0.0.1:9002", ""},
			}),
			want: "node 2 has an empty TLS identity",
		},
		{
			name: "Duplicate",
			nodes: WithNodes(map[ID]tlsNode{
				1: {"127.0.0.1:9001", "node1.relab.example"},
				2: {"127.0.0.1:9002", "node1.relab.example"},
			}),
			want: "nodes 1 and 2 have the same TLS identity",
		},
	}
	for _, tt := range tests {
		t.Run("NewConfig/"+tt.name, func(t *testing.T) {
			cfg, closeFn, err := NewConfig(tt.nodes, insecureDial)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewConfig() = %v, %v; want error containing %q", cfg, err, tt.want)
			}
			if cfg != nil || closeFn != nil {
				t.Errorf("NewConfig() = %v, %v; want nil configuration and close function", cfg, closeFn != nil)
			}
		})
		t.Run("Extend/"+tt.name, func(t *testing.T) {
			pool := WithNodes(map[ID]tlsNode{9: {"127.0.0.1:9109", "node9.relab.example"}})
			cfg, closeFn, err := NewConfig(pool, insecureDial)
			if err != nil {
				t.Fatalf("NewConfig: %v", err)
			}
			t.Cleanup(closeFn)
			if _, err := cfg.Extend(tt.nodes); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Extend() error = %v; want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestConfigExtendIdentity verifies that [Config.Extend] accepts a source only
// if it agrees with the pool on whether dialed servers are identity checked.
func TestConfigExtendIdentity(t *testing.T) {
	withIdentity := func(id ID, port int) NodeSource {
		return WithNodes(map[ID]tlsNode{id: {fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("node%d.relab.example", id)}})
	}
	addressOnly := WithNodeList([]string{"127.0.0.1:9200"})
	tests := []struct {
		name    string
		pool    NodeSource
		extend  NodeSource
		wantErr string
	}{
		{name: "IdentityWithIdentity", pool: withIdentity(1, 9101), extend: withIdentity(2, 9102)},
		{name: "AddressOnlyWithAddressOnly", pool: WithNodeList([]string{"127.0.0.1:9100"}), extend: addressOnly},
		{
			name:    "IdentityWithAddressOnly",
			pool:    withIdentity(1, 9101),
			extend:  addressOnly,
			wantErr: "with nodes without TLS identities",
		},
		{
			name:    "AddressOnlyWithIdentity",
			pool:    WithNodeList([]string{"127.0.0.1:9100"}),
			extend:  withIdentity(2, 9102),
			wantErr: "with nodes that have TLS identities",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, closeFn, err := NewConfig(tt.pool, insecureDial)
			if err != nil {
				t.Fatalf("NewConfig: %v", err)
			}
			t.Cleanup(closeFn)
			got, err := cfg.Extend(tt.extend)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Extend() error = %v; want it to contain %q", err, tt.wantErr)
				}
				if cfg.Size() != 1 {
					t.Errorf("configuration size = %d after a rejected Extend; want 1", cfg.Size())
				}
				return
			}
			if err != nil {
				t.Fatalf("Extend() error = %v; want nil", err)
			}
			if got.Size() != 2 {
				t.Errorf("extended configuration size = %d; want 2", got.Size())
			}
		})
	}
}
