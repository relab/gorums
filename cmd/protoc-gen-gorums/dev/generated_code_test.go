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
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
	"github.com/relab/gorums/gorumstest"
)

// quorumCallServer implements the QuorumCall and GRPCCall methods of the
// generated [dev.ZorumsServiceServer] interface. QuorumCall returns the length
// of the request value. GRPCCall returns the server's tag, which identifies
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

func (s quorumCallServer) GRPCCall(_ gorums.ServerContext, _ *dev.Request) (*dev.Response, error) {
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
		config   gorums.Config
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
	req.SetID(42)
	if got := req.GetID(); got != 42 {
		t.Errorf("GetID() = %d, want 42", got)
	}
	var _ dev.ID // the reserved alias must remain usable alongside the field
}

// TestGeneratedCodeClient checks the configuration access methods and the
// node calls of the generated ZorumsServiceClient type.
func TestGeneratedCodeClient(t *testing.T) {
	const numServers = 3
	config := gorumstest.Config(t, numServers, newQuorumCallServer)
	client := dev.NewZorumsServiceClient(config)

	t.Run("QuorumCall", func(t *testing.T) {
		req := &dev.Request{}
		req.SetValue("test")
		resp, err := client.QuorumCall(gorumstest.Context(t, 2*time.Second), req).Majority()
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
	if slices.Contains(client.Config().Nodes(), otherNode) {
		t.Fatalf("node %v is in the client's configuration", otherNode)
	}

	// A node call runs on the given node and ignores the client's
	// configuration, so a configuration client and a zero client reach the
	// same server, also for a node outside the configuration.
	clients := []struct {
		name   string
		client dev.ZorumsServiceClient
	}{
		{name: "ConfigClient", client: client},
		{name: "ZeroClient", client: dev.ZorumsServiceClient{}},
	}
	nodes := []struct {
		name    string
		node    *gorums.Node
		wantTag int64
	}{
		{name: "FirstNode", node: config.Nodes()[0], wantTag: 0},
		{name: "LastNode", node: config.Nodes()[numServers-1], wantTag: numServers - 1},
		{name: "OtherConfigNode", node: otherNode, wantTag: 100},
	}
	for _, c := range clients {
		for _, n := range nodes {
			t.Run("NodeCall/"+c.name+"/"+n.name, func(t *testing.T) {
				resp, err := c.client.GRPCCall(gorumstest.Context(t, 2*time.Second), n.node, &dev.Request{})
				if err != nil {
					t.Fatalf("GRPCCall failed: %v", err)
				}
				if got := resp.GetResult(); got != n.wantTag {
					t.Errorf("GRPCCall result = %d, want %d", got, n.wantTag)
				}
			})
		}
	}

	t.Run("ZeroClientQuorumCall", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("QuorumCall on a zero client did not panic")
			}
		}()
		dev.ZorumsServiceClient{}.QuorumCall(t.Context(), &dev.Request{})
	})

	t.Run("WithConfig", func(t *testing.T) {
		id := config.NodeIDs()[0]
		smaller := client.WithConfig(client.Config().Remove(id))
		if got, want := smaller.Config().Size(), numServers-1; got != want {
			t.Errorf("WithConfig(Remove(%d)).Config().Size() = %d, want %d", id, got, want)
		}
		if smaller.Config().Contains(id) {
			t.Errorf("WithConfig(Remove(%d)).Config() contains node %d", id, id)
		}
		if got := client.Config().Size(); got != numServers {
			t.Errorf("client.Config().Size() = %d after WithConfig, want %d", got, numServers)
		}
	})
}

// Sinks keep the compiler from eliding the operations measured in
// TestGeneratedCodeClientAllocs.
var (
	clientSink dev.ZorumsServiceClient
	configSink gorums.Config
)

// TestGeneratedCodeClientAllocs checks that constructing the generated client
// type and accessing its configuration allocates nothing.
func TestGeneratedCodeClientAllocs(t *testing.T) {
	config := gorumstest.Config(t, 3, newQuorumCallServer)
	smaller := config.Remove(config.NodeIDs()[0])
	client := dev.NewZorumsServiceClient(config)

	tests := []struct {
		name string
		fn   func()
	}{
		{name: "NewZorumsServiceClient", fn: func() { clientSink = dev.NewZorumsServiceClient(config) }},
		{name: "WithConfig", fn: func() { clientSink = client.WithConfig(smaller) }},
		{name: "Config", fn: func() { configSink = client.Config() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(1000, tt.fn); allocs != 0 {
				t.Errorf("%s allocates %v times per run, want 0", tt.name, allocs)
			}
		})
	}
}
