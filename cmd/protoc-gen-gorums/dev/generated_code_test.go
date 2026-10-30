// Package dev_test contains integration tests for the generated Gorums code.
// These tests validate that the protoc-gen-gorums code generator produces
// correct and functional code. They exercise the generated server
// registration, the generated QuorumCall method, and its terminal methods
// (Majority, All, Threshold) along with custom aggregation that ranges over
// the response sequence.
//
// NOTE: These tests are intentionally separate from the core library tests
// in the repository root. While they test similar functionality, they serve
// a different purpose: verifying the code generation pipeline end-to-end.
package dev_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
	"github.com/relab/gorums/gorumstest"
)

// quorumCallServer implements the QuorumCall and RemoteCall methods of the
// generated [dev.ZorumsServiceServer] interface. QuorumCall returns the length
// of the request value. RemoteCall returns the server's tag, which identifies
// the server that handled the call. The embedded interface is nil, so the
// other methods panic if called.
type quorumCallServer struct {
	dev.ZorumsServiceServer
	tag int64
}

func (quorumCallServer) QuorumCall(_ gorums.ServerContext, req *dev.Request) (*dev.Response, error) {
	resp := &dev.Response{}
	resp.SetResult(int64(len(req.GetValue())))
	return resp, nil
}

func (s quorumCallServer) RemoteCall(_ gorums.ServerContext, _ *dev.Request) (*dev.Response, error) {
	resp := &dev.Response{}
	resp.SetResult(s.tag)
	return resp, nil
}

func newQuorumCallServer(i int) gorumstest.ServerIface {
	return newTaggedServer(0)(i)
}

// newTaggedServer returns a server constructor whose i-th server has the tag
// base+i.
func newTaggedServer(base int64) func(int) gorumstest.ServerIface {
	return func(i int) gorumstest.ServerIface {
		srv := gorums.NewServer()
		dev.RegisterZorumsServiceServer(srv, quorumCallServer{tag: base + int64(i)})
		return srv
	}
}

func TestGeneratedCodeQuorumCall(t *testing.T) {
	tests := []struct {
		name  string
		value string
		call  func(context.Context, dev.ZorumsServiceClient, *dev.Request) (int64, error)
		want  int64
	}{
		{
			name:  "Majority",
			value: "test",
			call: func(ctx context.Context, client dev.ZorumsServiceClient, req *dev.Request) (int64, error) {
				resp, err := client.QuorumCall(ctx, req).Majority()
				return resp.GetResult(), err
			},
			want: 4,
		},
		{
			name:  "All",
			value: "test",
			call: func(ctx context.Context, client dev.ZorumsServiceClient, req *dev.Request) (int64, error) {
				resp, err := client.QuorumCall(ctx, req).All()
				return resp.GetResult(), err
			},
			want: 4,
		},
		{
			name:  "Threshold",
			value: "hello",
			call: func(ctx context.Context, client dev.ZorumsServiceClient, req *dev.Request) (int64, error) {
				resp, err := client.QuorumCall(ctx, req).Threshold(2)
				return resp.GetResult(), err
			},
			want: 5,
		},
		{
			// Each of the 3 servers returns len("hello"), so the sum is 15.
			name:  "ResultsSum",
			value: "hello",
			call: func(ctx context.Context, client dev.ZorumsServiceClient, req *dev.Request) (int64, error) {
				var total int64
				for resp := range client.QuorumCall(ctx, req).Results().IgnoreErrors() {
					total += resp.Value.GetResult()
				}
				return total, nil
			},
			want: 15,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dev.NewZorumsServiceClient(gorumstest.Config(t, 3, newQuorumCallServer))
			ctx := gorumstest.Context(t, 2*time.Second)

			req := &dev.Request{}
			req.SetValue(tt.value)
			got, err := tt.call(ctx, client, req)
			if err != nil {
				t.Fatalf("QuorumCall failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("QuorumCall result = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestGeneratedCodeDerivedConfig checks that the generated QuorumCall works on
// configurations derived with Remove, Union, and Difference.
func TestGeneratedCodeDerivedConfig(t *testing.T) {
	base := gorumstest.Config(t, 6, newQuorumCallServer)
	removed := base.Remove(1, 2)
	union := base.Union(removed)

	tests := []struct {
		name     string
		config   dev.Config
		wantSize int
	}{
		{name: "Base", config: base, wantSize: 6},
		{name: "Remove", config: removed, wantSize: 4},
		{name: "Union", config: union, wantSize: 6},
		{name: "Difference", config: union.Difference(removed), wantSize: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Size(); got != tt.wantSize {
				t.Fatalf("config.Size() = %d, want %d", got, tt.wantSize)
			}
			client := dev.NewZorumsServiceClient(tt.config)
			ctx := gorumstest.Context(t, 2*time.Second)
			req := &dev.Request{}
			req.SetValue("test")

			resp, err := client.QuorumCall(ctx, req).Majority()
			if err != nil {
				t.Fatalf("QuorumCall failed: %v", err)
			}
			if got := resp.GetResult(); got != 4 {
				t.Errorf("QuorumCall result = %d, want 4", got)
			}

			replies := 0
			for range client.QuorumCall(ctx, req).Results() {
				replies++
			}
			if replies != tt.wantSize {
				t.Errorf("Results yielded %d replies, want %d", replies, tt.wantSize)
			}
		})
	}
}

// TestGeneratedCodeIDField verifies that a proto message can have a field
// named ID, even though ID is a reserved identifier in the generated package.
func TestGeneratedCodeIDField(t *testing.T) {
	req := &dev.Request{}
	req.SetId(42)
	if got := req.GetId(); got != 42 {
		t.Errorf("GetId() = %d, want 42", got)
	}
	var _ dev.ID // the reserved alias must remain usable alongside the field
}

// TestGeneratedCodeClient checks the quorum calls of the generated
// ZorumsServiceClient and the node calls of the generated
// ZorumsServiceNodeClient.
func TestGeneratedCodeClient(t *testing.T) {
	const numServers = 3
	config := gorumstest.Config(t, numServers, newQuorumCallServer)

	t.Run("QuorumCall", func(t *testing.T) {
		req := &dev.Request{}
		req.SetValue("test")
		resp, err := dev.NewZorumsServiceClient(config).QuorumCall(gorumstest.Context(t, 2*time.Second), req).Majority()
		if err != nil {
			t.Fatalf("QuorumCall failed: %v", err)
		}
		if got := resp.GetResult(); got != 4 {
			t.Errorf("QuorumCall result = %d, want 4", got)
		}
	})

	// otherNode belongs to a second configuration; its server's tag is 100.
	// The goroutine leak check of the first configuration runs last and
	// covers both, so the second skips its own check, which would otherwise
	// run while the first configuration is still open.
	otherNode := gorumstest.Node(t, newTaggedServer(100), gorumstest.SkipGoleak())
	if slices.Contains(config.Nodes(), otherNode) {
		t.Fatalf("node %v is in the configuration", otherNode)
	}

	// A node client runs its calls on its node, which may belong to any
	// configuration.
	nodes := []struct {
		name    string
		node    *dev.Node
		wantTag int64
	}{
		{name: "FirstNode", node: config.Nodes()[0], wantTag: 0},
		{name: "LastNode", node: config.Nodes()[numServers-1], wantTag: numServers - 1},
		{name: "OtherConfigNode", node: otherNode, wantTag: 100},
	}
	for _, n := range nodes {
		t.Run("NodeCall/"+n.name, func(t *testing.T) {
			nodeClient := dev.NewZorumsServiceNodeClient(n.node)
			resp, err := nodeClient.RemoteCall(gorumstest.Context(t, 2*time.Second), &dev.Request{})
			if err != nil {
				t.Fatalf("RemoteCall failed: %v", err)
			}
			if got := resp.GetResult(); got != n.wantTag {
				t.Errorf("RemoteCall result = %d, want %d", got, n.wantTag)
			}
		})
	}

	// A node client of a nil node, such as the node that Config.Node returns
	// for an unknown ID, reports ErrNoNode from every call.
	nilNodeClient := dev.NewZorumsServiceNodeClient(config.Node(numServers + 100))
	nilNodeCalls := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "RemoteCall",
			call: func(ctx context.Context) error {
				_, err := nilNodeClient.RemoteCall(ctx, &dev.Request{})
				return err
			},
		},
		{
			name: "UnicastSend",
			call: func(ctx context.Context) error { return nilNodeClient.Unicast(ctx, &dev.Request{}).Send() },
		},
		{
			name: "UnicastAsync",
			call: func(ctx context.Context) error { return nilNodeClient.Unicast(ctx, &dev.Request{}).Async().Wait() },
		},
	}
	for _, c := range nilNodeCalls {
		t.Run("NilNode/"+c.name, func(t *testing.T) {
			if err := c.call(t.Context()); !errors.Is(err, gorums.ErrNoNode) {
				t.Errorf("call error = %v, want %v", err, gorums.ErrNoNode)
			}
		})
	}

	// A configuration client of an empty configuration reports
	// ErrEmptyConfig from every call.
	emptyClient := dev.NewZorumsServiceClient(nil)
	emptyConfigCalls := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "QuorumCall",
			call: func(ctx context.Context) error {
				_, err := emptyClient.QuorumCall(ctx, &dev.Request{}).Majority()
				return err
			},
		},
		{
			name: "MulticastSend",
			call: func(ctx context.Context) error { return emptyClient.MulticastWithResponse(ctx, &dev.Request{}).Send() },
		},
	}
	for _, c := range emptyConfigCalls {
		t.Run("EmptyConfig/"+c.name, func(t *testing.T) {
			if err := c.call(t.Context()); !errors.Is(err, gorums.ErrEmptyConfig) {
				t.Errorf("call error = %v, want %v", err, gorums.ErrEmptyConfig)
			}
		})
	}
}

// Sinks keep the compiler from eliding the operations measured in
// TestGeneratedCodeClientAllocs.
var (
	clientSink     dev.ZorumsServiceClient
	nodeClientSink dev.ZorumsServiceNodeClient
)

// TestGeneratedCodeClientAllocs checks how often constructing the generated
// clients allocates. A node client holds only a pointer, so building one
// allocates nothing. A configuration client holds a slice, which is stored on
// the heap behind the interface.
func TestGeneratedCodeClientAllocs(t *testing.T) {
	config := gorumstest.Config(t, 3, newQuorumCallServer)
	node := config.Nodes()[0]

	tests := []struct {
		name string
		fn   func()
		want float64
	}{
		{name: "NewZorumsServiceClient", fn: func() { clientSink = dev.NewZorumsServiceClient(config) }, want: 1},
		{name: "NewZorumsServiceNodeClient", fn: func() { nodeClientSink = dev.NewZorumsServiceNodeClient(node) }, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(1000, tt.fn); allocs != tt.want {
				t.Errorf("%s allocates %v times per run, want %v", tt.name, allocs, tt.want)
			}
		})
	}
}
