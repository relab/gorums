package gorums_test

import (
	"errors"
	"fmt"
	"strings"
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
}

func TestSystemStreamDeduplicated(t *testing.T) {
	t.Skip("Temporarily skipping since I've rolled back the stream deduplication changes. Will re-enable once we reintroduce stream deduplication.")
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
}

func TestSystemSymmetricQuorumCallFromHandler_Config(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	for i, sys := range systems {
		myID := i + 1
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				req := gorums.AsProto[*pb.StringValue](in)
				t.Logf("System %d received request: %s", myID, req.GetValue())
				if req.GetValue() == "inner-call" {
					return gorums.NewResponseMessage(in, pb.String("inner-echo: "+req.GetValue())), nil
				}

				cfg := ctx.Config()
				if cfg == nil || cfg.Size() != 3 {
					return nil, errors.New("expected config size 3")
				}

				// Release the NodeStream mutex before making the inner quorum call.
				// Without this, the NodeStream's Recv loop cannot read the inner-call
				// responses off the wire while this handler is blocked waiting for them,
				// causing a deadlock.
				ctx.Release()

				responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfg.Context(ctx.Context),
					pb.String("inner-call"),
					mock.TestMethod,
				)
				res, err := responses.Majority()
				if err != nil {
					return nil, err
				}

				resp := pb.String(fmt.Sprintf("sys-%d echoes: %s | %s", myID, req.GetValue(), res.GetValue()))
				return gorums.NewResponseMessage(in, resp), nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := configs[0]
	ctx := gorums.TestContext(t, 2*time.Second)

	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfg.Context(ctx),
		pb.String("outer-call"),
		mock.TestMethod,
	)
	result, err := responses.Majority()
	if err != nil {
		t.Fatalf("quorum call error: %v", err)
	}
	t.Logf("Final result: %s", result.GetValue())

	wantResult := "echoes: outer-call | inner-echo: inner-call"
	if strings.Contains(result.GetValue(), wantResult) {
		t.Logf("Found expected result: %s", result.GetValue())
	} else {
		t.Errorf("Expected %s in result, got: %s", wantResult, result.GetValue())
	}
}

func TestSystemSymmetricMulticastFromHandler_Config(t *testing.T) {
	systems, configs := createTestSystems(t, 3)

	// 3 servers receive the outer multicast. each server multicasts to a config of 3 nodes.
	// however, self-node enqueues are silently dropped in gorums, so each server only sends to 2 peers.
	// total = 3 * 2 = 6 messages received.
	var wg sync.WaitGroup
	wg.Add(6)

	for i, sys := range systems {
		sys.RegisterService(configs[i].Manager(), func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				t.Logf("System %d received multicast on %v: %v", i+1, mock.TestMethod, in.Msg)
				if cfg := ctx.Config(); cfg != nil && cfg.Size() == 3 {
					_ = gorums.Multicast(
						cfg.Context(t.Context()),
						pb.String("inner-multicast"),
						mock.Stream,
					)
				}
				return nil, nil // one-way
			})

			srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				t.Logf("System %d received multicast on %v: %v", i+1, mock.Stream, in.Msg)
				wg.Done()
				return nil, nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := configs[0]
	ctx := gorums.TestContext(t, 2*time.Second)
	_ = gorums.Multicast(
		cfg.Context(ctx),
		pb.String("outer-multicast"),
		mock.TestMethod,
	)

	waitWithTimeout(t, &wg)
}

func TestSystemSymmetricQuorumCallFromHandler_ClientConfig(t *testing.T) {
	systems, configs := gorums.TestSystems(t, 2, func(i int, addrs []string) ([]gorums.ServerOption, []gorums.Option) {
		if i == 0 {
			return []gorums.ServerOption{gorums.WithClientConfig()}, []gorums.Option{gorums.WithNodeList([]string{addrs[0]}), gorums.InsecureDialOptions(t)}
		}
		// System 1 connects to System 0
		return []gorums.ServerOption{gorums.WithClientConfig()}, []gorums.Option{gorums.WithNodeList([]string{addrs[0]}), gorums.InsecureDialOptions(t)}
	})

	sysServer := systems[0]
	sysServer.RegisterService(configs[0].Manager(), func(srv *gorums.Server) {
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*pb.StringValue](in)
			t.Logf("SERVER received quorum call on %v: %v", mock.TestMethod, req.GetValue())
			if req.GetValue() == "inner-call" {
				return gorums.NewResponseMessage(in, pb.String("inner-echo: "+req.GetValue())), nil
			}

			cfg := ctx.ClientConfig()
			if cfg == nil || cfg.Size() != 2 {
				return nil, errors.New("expected client config size 2")
			}

			// Release the NodeStream mutex before making the inner quorum call.
			// The ClientConfig has 2 nodes: system 0's self-connection and system 1.
			// System 1's inner response arrives on the same NodeStream that is
			// currently blocked waiting for this handler — releasing first prevents
			// a deadlock if system 1 responds before the self-connection does.
			ctx.Release()

			responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx.Context),
				pb.String("inner-call"),
				mock.TestMethod, // sent back to the two client peers
			)
			// First() returns whichever client responds first:
			//   - system 0 self-connection → "inner-echo: inner-call"
			//   - system 1 → "client-echoed: inner-call"
			res, err := responses.First()
			if err != nil {
				return nil, err
			}
			t.Logf("SERVER final result from %v: %v", mock.TestMethod, res.GetValue())
			return gorums.NewResponseMessage(in, pb.String(req.GetValue()+" | "+res.GetValue())), nil
		})
	})

	sysClient := systems[1]
	sysClient.RegisterService(configs[1].Manager(), func(srv *gorums.Server) {
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*pb.StringValue](in)
			t.Logf("CLIENT received quorum call on %v: %v", mock.TestMethod, req.GetValue())
			return gorums.NewResponseMessage(in, pb.String("client-echoed: "+req.GetValue())), nil
		})
	})

	gorums.WaitForConfigCondition(t, sysServer.ClientConfig, func(cfg gorums.Configuration) bool { return cfg.Size() == 2 })

	// Trigger the outer call from client to server
	cfgClient := configs[1]
	ctx := gorums.TestContext(t, 2*time.Second)
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfgClient.Context(ctx),
		pb.String("outer-call"),
		mock.TestMethod,
	)
	result, err := responses.First()
	if err != nil {
		t.Fatalf("quorum call error: %v", err)
	}
	t.Logf("CLIENT final result from %v: %v", mock.TestMethod, result.GetValue())
	// The result prefix is fixed; the suffix depends on which client peer responded
	// first to the inner call (self-connection vs system 1 — both are valid).
	if !strings.HasPrefix(result.GetValue(), "outer-call | ") {
		t.Errorf("Expected result to start with %q, got %q", "outer-call | ", result.GetValue())
	}
}

func TestSystemSymmetricMulticastFromHandler_ClientConfig(t *testing.T) {
	systems, configs := gorums.TestSystems(t, 2, func(i int, addrs []string) ([]gorums.ServerOption, []gorums.Option) {
		if i == 0 {
			return []gorums.ServerOption{gorums.WithClientConfig()}, []gorums.Option{gorums.WithNodeList([]string{addrs[0]}), gorums.InsecureDialOptions(t)}
		}
		return []gorums.ServerOption{gorums.WithClientConfig()}, []gorums.Option{gorums.WithNodeList([]string{addrs[0]}), gorums.InsecureDialOptions(t)}
	})

	// Outer multicast from client triggers 1 server handler.
	// That server handler multicasts to config of size 2 -> 2 messages sent.
	// So wg expects 2 receives.
	var wg sync.WaitGroup
	wg.Add(2)

	sysServer := systems[0]
	sysServer.RegisterService(configs[0].Manager(), func(srv *gorums.Server) {
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("SERVER received multicast on %v: %v", mock.TestMethod, in.Msg)
			if cfg := ctx.ClientConfig(); cfg != nil && cfg.Size() == 2 {
				_ = gorums.Multicast(
					cfg.Context(t.Context()),
					pb.String("inner-call"),
					mock.Stream,
				)
			}
			return nil, nil // one-way
		})
		srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("SERVER received multicast on %v: %v", mock.Stream, in.Msg)
			wg.Done()
			return nil, nil
		})
	})

	sysClient := systems[1]
	sysClient.RegisterService(configs[1].Manager(), func(srv *gorums.Server) {
		srv.RegisterHandler(mock.Stream, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("CLIENT received multicast on %v: %v", mock.Stream, in.Msg)
			wg.Done()
			return nil, nil
		})
	})

	gorums.WaitForConfigCondition(t, sysServer.ClientConfig, func(cfg gorums.Configuration) bool { return cfg.Size() == 2 })

	cfgClient := configs[1]
	ctx := gorums.TestContext(t, 2*time.Second)
	_ = gorums.Multicast(
		cfgClient.Context(ctx),
		pb.String("trigger"),
		mock.TestMethod,
	)

	waitWithTimeout(t, &wg)
}
