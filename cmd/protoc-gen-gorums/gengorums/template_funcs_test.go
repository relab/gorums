package gengorums

import (
	"maps"
	"testing"
)

func TestTemplateFuncsLocalNames(t *testing.T) {
	tests := []struct {
		name  string
		bases string
		refs  []any
		want  map[string]string
	}{
		{
			name:  "NoCollision",
			bases: "c ctx in",
			refs:  []any{"pb.Request", "context.Context", "gorumsimpl.QuorumCall"},
			want:  map[string]string{"c": "c", "ctx": "ctx", "in": "in"},
		},
		{
			name:  "QualifiedRef",
			bases: "c ctx node in",
			refs:  []any{"node.Request", "c.Response"},
			want:  map[string]string{"c": "c_", "ctx": "ctx", "node": "node_", "in": "in"},
		},
		{
			name:  "SliceAndUnqualifiedRefs",
			bases: "srv impl",
			refs:  []any{[]string{"srv.Request", "srv_.Response"}, "impl"},
			want:  map[string]string{"srv": "srv__", "impl": "impl_"},
		},
		{
			name:  "LocalNamesDiffer",
			bases: "in in_",
			refs:  []any{"in.Request"},
			want:  map[string]string{"in": "in_", "in_": "in__"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localNames(tt.bases, tt.refs...); !maps.Equal(got, tt.want) {
				t.Errorf("localNames(%q, %v) = %v, want %v", tt.bases, tt.refs, got, tt.want)
			}
		})
	}
}
