package gorums_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

type mockCloser struct {
	closed bool
	err    error
}

func (m *mockCloser) Close() error {
	m.closed = true
	return m.err
}

func TestSystemLifecycle(t *testing.T) {
	sys, err := gorums.NewSystem("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	closer1 := &mockCloser{}
	closer2 := &mockCloser{}

	sys.RegisterService(closer1, func(*gorums.Server) {
		// In a real scenario, we would register a Gorums service here.
	})
	sys.RegisterService(closer2, func(*gorums.Server) {
		// Register another service or just use the callback.
	})

	go func() {
		// Serve acts as a blocking call, so run in goroutine
		if err := sys.Serve(); err != nil {
			// Serve returns error on Stop usually (or net closed)
			t.Logf("Serve returned: %v", err)
		}
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop the system
	if err := sys.Stop(); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}

	if !closer1.closed {
		t.Error("closer1 was not closed")
	}
	if !closer2.closed {
		t.Error("closer2 was not closed")
	}
}

func TestSystemStopError(t *testing.T) {
	sys, err := gorums.NewSystem("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	errCloser := &mockCloser{err: errors.New("closer error")}

	sys.RegisterService(errCloser, func(*gorums.Server) {})

	go func() {
		_ = sys.Serve()
	}()
	time.Sleep(10 * time.Millisecond)

	err = sys.Stop()
	if err == nil {
		t.Error("expected error from Stop, got nil")
	}
}

func TestSystemSymmetricConfiguration(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	// Each replica connects to the other two via System.NewOutboundConfig
	// (NodeID is automatically included in metadata)
	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(*gorums.Server) {
			// Register mock handlers for the server sides if needed for other tests
		})
	}

	// Wait for connections to establish
	for i, sys := range systems {
		gorums.WaitForConfigCondition(t, sys.Config, func(cfg gorums.Configuration) bool {
			return cfg.Size() == len(systems)
		})
		if got := sys.Config().Size(); got != len(systems) {
			t.Fatalf("system %d config size: %d, expected: %d", i+1, got, len(systems))
		}
	}
}

func createTestSystems(t *testing.T, numSystems int) ([]*gorums.System, []gorums.Configuration) {
	systems, configs := gorums.TestSystems(t, numSystems, func(i int, addrs []string) ([]gorums.ServerOption, []gorums.Option) {
		myID := uint32(i + 1)

		nodeList := gorums.WithNodeList(addrs)
		srvOpts := []gorums.ServerOption{
			gorums.WithConfig(myID, nodeList),
		}

		cfgOpts := []gorums.Option{
			nodeList,
			gorums.InsecureDialOptions(t),
		}

		return srvOpts, cfgOpts
	})
	return systems, configs
}

// waitWithTimeout waits for wg to reach zero or calls t.Fatal if the timeout elapses.
func waitWithTimeout(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for handlers to be invoked")
	}
}

func awaitSystemReady(t *testing.T, systems []*gorums.System) {
	t.Helper()
	for _, sys := range systems {
		gorums.WaitForConfigCondition(t, sys.Config, func(cfg gorums.Configuration) bool {
			return cfg.Size() == len(systems)
		})
	}
}

func TestSystemSymmetricConfigurationQuorumCall(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	// Register mock handler to each system
	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				req := gorums.AsProto[*pb.StringValue](in)
				resp := pb.String("echo: " + req.GetValue())
				return gorums.NewResponseMessage(in, resp), nil
			})
		})
	}

	awaitSystemReady(t, systems)

	// type alias short hand for the responses type
	type respType = *gorums.Responses[*pb.StringValue]
	tests := []struct {
		name      string
		call      func(respType) (*pb.StringValue, error)
		wantValue string
	}{
		{
			name:      "Majority",
			call:      respType.Majority,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      respType.First,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      respType.All,
			wantValue: "echo: test",
		},
	}

	// Use the explicit config from node 1 outbound.
	cfg := configs[0]
	// Sub tests for each response type logic across symmetric routing
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := gorums.TestContext(t, 2*time.Second)

			responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String("test"),
				mock.TestMethod,
			)

			result, err := tt.call(responses)
			if err != nil {
				t.Fatalf("quorum call error: %v", err)
			}

			if result.GetValue() != tt.wantValue {
				t.Errorf("Expected %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

func TestSystemSymmetricConfigurationMulticast(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	var wg sync.WaitGroup
	wg.Add(len(systems))

	// Register mock handler to each system
	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.Stream, func(_ gorums.ServerCtx, _ *gorums.Message) (*gorums.Message, error) {
				wg.Done()
				return nil, nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := configs[0]

	t.Run("Multicast", func(t *testing.T) {
		ctx := gorums.TestContext(t, 2*time.Second)
		err := gorums.Multicast(
			cfg.Context(ctx),
			pb.String("test"),
			mock.Stream,
		)
		if err != nil {
			t.Fatalf("multicast error: %v", err)
		}

		waitWithTimeout(t, &wg)
	})
}

func TestSystemStreamDedupQuorumCall(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	// Register echo handler to each system
	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				req := gorums.AsProto[*pb.StringValue](in)
				resp := pb.String("echo: " + req.GetValue())
				return gorums.NewResponseMessage(in, resp), nil
			})
		})
	}

	awaitSystemReady(t, systems)

	// Verify stream dedup: for each peer pair, exactly one side should have
	// an outbound channel. The lower-ID node keeps its outbound.
	for i, cfg := range configs {
		myID := uint32(i + 1)
		for _, node := range cfg.Nodes() {
			peerID := node.ID()
			if peerID == myID {
				continue // skip self
			}
			ch := gorums.TestNodeChannel(node)
			if myID < peerID {
				// Lower-ID node: should have outbound channel (conn != nil)
				if ch == nil {
					t.Errorf("system %d -> peer %d: expected outbound channel; got nil", myID, peerID)
				} else if ch.IsInbound() {
					t.Errorf("system %d -> peer %d: expected outbound channel; got inbound", myID, peerID)
				}
			} else {
				// Higher-ID node: should have no outbound channel (waiting for inbound)
				if ch != nil && !ch.IsInbound() {
					t.Errorf("system %d -> peer %d: expected no outbound channel; got outbound", myID, peerID)
				}
			}
		}
	}

	// Verify quorum calls still work across deduplicated streams
	cfg := configs[0]
	ctx := gorums.TestContext(t, 2*time.Second)

	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfg.Context(ctx),
		pb.String("dedup-test"),
		mock.TestMethod,
	)

	result, err := responses.Majority()
	if err != nil {
		t.Fatalf("quorum call error: %v", err)
	}
	if result.GetValue() != "echo: dedup-test" {
		t.Errorf("Expected %q, got %q", "echo: dedup-test", result.GetValue())
	}
}

func TestSystemStreamDedupMulticast(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	var wg sync.WaitGroup
	wg.Add(len(systems))

	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.Stream, func(_ gorums.ServerCtx, _ *gorums.Message) (*gorums.Message, error) {
				wg.Done()
				return nil, nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := configs[0]
	ctx := gorums.TestContext(t, 2*time.Second)
	err := gorums.Multicast(
		cfg.Context(ctx),
		pb.String("dedup-multicast"),
		mock.Stream,
	)
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}
