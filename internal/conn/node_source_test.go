package conn

import "testing"

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
