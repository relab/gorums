package conn

import (
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func insecureDial(o *DialOptions) {
	o.GRPCDialOptions = append(o.GRPCDialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
			cfg, closeFn, err := NewConfig(WithNodeList([]string{"127.0.0.1:9100"}), insecureDial)
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
