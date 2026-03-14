package gorums_test

import (
	"errors"
	"net"
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

func TestSystemStopClosesRegisteredServices(t *testing.T) {
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

func TestSystemStopReturnsCloserError(t *testing.T) {
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

// TestSystemStopBeforeServeClosesListener verifies that Stop closes the
// pre-allocated listener even when Serve was never called, so no file
// descriptor is leaked.
func TestSystemStopBeforeServeClosesListener(t *testing.T) {
	sys, err := gorums.NewSystem("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	addr := sys.Addr()
	if err := sys.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// The listener should be closed now; re-binding to the same address must succeed.
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Errorf("expected to re-bind to %s after Stop (without Serve), got: %v", addr, err)
		return
	}
	_ = lis.Close()
}

// TestNewLocalSystemsStopBeforeServeClosesListeners verifies that the stop function
// returned by NewLocalSystems closes all pre-allocated listeners even when none of
// the systems has had Serve called yet, so no file descriptors are leaked.
func TestNewLocalSystemsStopBeforeServeClosesListeners(t *testing.T) {
	systems, stop, err := gorums.NewLocalSystems(3, gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewLocalSystems: %v", err)
	}
	addrs := make([]string, len(systems))
	for i, sys := range systems {
		addrs[i] = sys.Addr()
	}
	stop() // called before any Serve()
	// Every pre-allocated listener must be closed; re-binding must succeed.
	for _, addr := range addrs {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("expected to re-bind to %s after stop (without Serve), got: %v", addr, err)
			continue
		}
		_ = lis.Close()
	}
}

func TestSystemSymmetricConfigurationConnectsAllPeers(t *testing.T) {
	systems := gorums.TestSystems(t, 3)

	// Outbound config is auto-created by NewLocalSystems.
	// (NodeID is automatically included in connection metadata)
	for _, sys := range systems {
		sys.RegisterService(nil, func(*gorums.Server) {
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

// awaitClientReady waits until the server's ClientConfig contains n connected peers.
func awaitClientReady(t *testing.T, sys *gorums.System, n int) {
	t.Helper()
	gorums.WaitForConfigCondition(t, sys.ClientConfig, func(cfg gorums.Configuration) bool {
		return cfg.Size() == n
	})
}

// createClientServerSystems creates a server system and a client for back-channel testing.
// The server automatically tracks anonymous clients and can dispatch reverse-direction
// calls to them via [ServerCtx.ClientConfig].
// The client is a standalone [*gorums.Server] (no listener needed) whose registered handlers
// are reachable by the server over the existing bidirectional gRPC stream. The returned
// [gorums.Configuration] is the client's outbound config pointing at the server.
func createClientServerSystems(t *testing.T) (*gorums.System, *gorums.Server, gorums.Configuration) {
	t.Helper()

	// Server side: accepts anonymous clients for reverse-direction calls.
	sys, err := gorums.NewSystem("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// Client side: a plain Server whose handlers the server can invoke via the back-channel.
	// No listener is required — dispatch is over the client's outbound gRPC stream.
	clientSrv := gorums.NewServer()

	// The client dials the server; NewConfig on the server wires up the back-channel dispatcher.
	nodeList := gorums.WithNodeList([]string{sys.Addr()})
	cfg, err := clientSrv.NewConfig(nodeList, gorums.InsecureDialOptions(t))
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = sys.Serve() }()

	t.Cleanup(func() {
		_ = cfg.Manager().Close()
		_ = sys.Stop()
	})

	return sys, clientSrv, cfg
}

// stringEchoHandler returns a handler that replies with prefix+": "+request value.
func stringEchoHandler(prefix string) gorums.Handler {
	return func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, pb.String(prefix+": "+req.GetValue())), nil
	}
}

func configContext(ctx gorums.ServerCtx, client bool) (*gorums.ConfigContext, error) {
	if client {
		configContext := ctx.ClientConfigContext()
		if configContext == nil {
			return nil, errors.New("ClientConfigContext: expected non-nil config")
		}
		return configContext, nil
	}
	configContext := ctx.ConfigContext()
	if configContext == nil {
		return nil, errors.New("ConfigContext: expected non-nil config")
	}
	return configContext, nil
}

// outerChainedHandler returns an outer handler that fans out an inner quorum call
// on innerMethod, then combines the outer request value with the inner result.
// Unlike innerQuorumCallHandler, routing is done entirely by method registration:
// no message-content inspection is needed.
func outerChainedHandler(
	t *testing.T,
	myID int,
	client bool,
	innerMethod string,
	respFn func(*gorums.Responses[*pb.StringValue]) (*pb.StringValue, error),
) gorums.Handler {
	t.Helper()
	return func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		t.Logf("System %d received outer request: %s", myID, req.GetValue())
		// Release the NodeStream mutex before making the inner quorum call.
		// Without this, the NodeStream's Recv loop cannot read the inner-call
		// responses off the wire while this handler is blocked waiting for them,
		// causing a deadlock.
		ctx.Release()
		configCtx, err := configContext(ctx, client)
		if err != nil {
			return nil, err
		}
		responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
			configCtx,
			req,
			innerMethod,
		)
		res, err := respFn(responses)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, pb.String(req.GetValue()+" | "+res.GetValue())), nil
	}
}

func TestSystemSymmetricConfigurationRoutesQuorumCalls(t *testing.T) {
	systems := gorums.TestSystems(t, 3)

	// Register mock handler to each system
	for _, sys := range systems {
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, stringEchoHandler("echo"))
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

	// Use the auto-created outbound config from system 0.
	cfg := systems[0].OutboundConfig()
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

func TestSystemSymmetricConfigurationRoutesMulticast(t *testing.T) {
	systems := gorums.TestSystems(t, 3)

	var wg sync.WaitGroup
	wg.Add(len(systems))

	// Register mock handler to each system
	for _, sys := range systems {
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterHandler(mock.Stream, func(_ gorums.ServerCtx, _ *gorums.Message) (*gorums.Message, error) {
				wg.Done()
				return nil, nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := systems[0].OutboundConfig()
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

func TestSystemHandlerCanMulticastViaConfig(t *testing.T) {
	systems := gorums.TestSystems(t, 3)

	// 3 servers receive the outer multicast. Each server multicasts to a config of 3 nodes.
	// The self-node's handler is invoked locally, so each server sends to all 3 nodes.
	// Total = 3 * 3 = 9 messages received.
	var wg sync.WaitGroup
	wg.Add(9)

	for i, sys := range systems {
		sys.RegisterService(nil, func(srv *gorums.Server) {
			srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				t.Logf("System %d received multicast on %v: %v", i+1, mock.TestMethod, in.Msg)
				if cfg := ctx.Config(); cfg != nil && cfg.Size() == 3 {
					err := gorums.Multicast(
						cfg.Context(t.Context()),
						pb.String("inner-multicast"),
						mock.Stream,
					)
					if err != nil {
						return nil, err // failed to multicast
					}
				}
				return nil, nil // one-way
			})

			srv.RegisterHandler(mock.Stream, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
				t.Logf("System %d received multicast on %v: %v", i+1, mock.Stream, in.Msg)
				wg.Done()
				return nil, nil
			})
		})
	}

	awaitSystemReady(t, systems)

	cfg := systems[0].OutboundConfig()
	ctx := gorums.TestContext(t, 2*time.Second)
	err := gorums.Multicast(
		cfg.Context(ctx),
		pb.String("outer-multicast"),
		mock.TestMethod,
	)
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}

func TestSystemHandlerCanChainQuorumCallViaConfig(t *testing.T) {
	type respType = *gorums.Responses[*pb.StringValue]

	// seqAll drains the Seq iterator to exhaustion and returns the last value.
	// This is the regression path for the self-node dispatch bug where .Seq()
	// and .All() would time out when self was included in the quorum.
	seqAll := func(r respType) (*pb.StringValue, error) {
		var last *pb.StringValue
		for result := range r.Seq() {
			if result.Err != nil {
				return nil, result.Err
			}
			last = result.Value
		}
		if last == nil {
			return nil, errors.New("Seq: no responses received")
		}
		return last, nil
	}

	tests := []struct {
		name    string
		innerFn func(respType) (*pb.StringValue, error)
		outerFn func(respType) (*pb.StringValue, error)
	}{
		{name: "Majority", innerFn: respType.Majority, outerFn: respType.Majority},
		{name: "All", innerFn: respType.All, outerFn: respType.All}, // Regression: .All() must not time out when self-node is included.
		{name: "Seq", innerFn: seqAll, outerFn: respType.All},       // Regression: draining .Seq() to exhaustion must not time out when self-node is included.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			systems := gorums.TestSystems(t, 3)

			for i, sys := range systems {
				myID := i + 1
				sys.RegisterService(nil, func(srv *gorums.Server) {
					srv.RegisterHandler(mock.TestMethod, outerChainedHandler(t, myID, false, mock.EchoMethod, tt.innerFn))
					srv.RegisterHandler(mock.EchoMethod, stringEchoHandler("inner-echo"))
				})
			}

			awaitSystemReady(t, systems)

			cfg := systems[0].OutboundConfig()
			ctx := gorums.TestContext(t, 2*time.Second)

			responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String("outer-call"),
				mock.TestMethod,
			)
			result, err := tt.outerFn(responses)
			if err != nil {
				t.Fatalf("quorum call error: %v", err)
			}
			t.Logf("Final result: %s", result.GetValue())

			wantResult := "outer-call | inner-echo: outer-call"
			if !strings.Contains(result.GetValue(), wantResult) {
				t.Errorf("Expected %q in result, got: %s", wantResult, result.GetValue())
			}
		})
	}
}

func TestSystemHandlerCanChainQuorumCallViaClientConfig(t *testing.T) {
	sysServer, clientSrv, cfgClient := createClientServerSystems(t)

	// Server: outer handler fans out an inner quorum call on EchoMethod to all
	// client peers and returns whichever responds first.
	sysServer.RegisterService(nil, func(srv *gorums.Server) {
		srv.RegisterHandler(mock.TestMethod, outerChainedHandler(t, 1, true, mock.EchoMethod, (*gorums.Responses[*pb.StringValue]).First))
	})

	// Client: handles EchoMethod calls dispatched back by the server via ClientConfig.
	clientSrv.RegisterHandler(mock.EchoMethod, stringEchoHandler("client-echo"))

	awaitClientReady(t, sysServer, 1)

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
	t.Logf("CLIENT final result: %v", result.GetValue())
	// The server fans out EchoMethod to ClientConfig (client only).
	// Result: "outer-call | client-echo: outer-call"
	if !strings.HasPrefix(result.GetValue(), "outer-call | ") {
		t.Errorf("Expected result to start with %q, got %q", "outer-call | ", result.GetValue())
	}
}

func TestSystemHandlerCanMulticastViaClientConfig(t *testing.T) {
	sysServer, clientSrv, cfgClient := createClientServerSystems(t)

	// Outer multicast from client triggers the server handler once.
	// The server fans out an inner multicast via ClientConfig (1 client) -> 1 message.
	var wg sync.WaitGroup
	wg.Add(1)

	sysServer.RegisterService(nil, func(srv *gorums.Server) {
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			t.Logf("SERVER received multicast: %v", in.Msg)
			if cfg := ctx.ClientConfig(); cfg != nil && cfg.Size() == 1 {
				err := gorums.Multicast(
					cfg.Context(t.Context()),
					pb.String("inner-call"),
					mock.Stream,
				)
				if err != nil {
					return nil, err // failed to multicast
				}
			}
			return nil, nil // one-way
		})
	})

	// Client handles the reverse-direction multicast dispatched by the server.
	clientSrv.RegisterHandler(mock.Stream, func(_ gorums.ServerCtx, _ *gorums.Message) (*gorums.Message, error) {
		t.Log("CLIENT received inner multicast")
		wg.Done()
		return nil, nil
	})

	awaitClientReady(t, sysServer, 1)

	ctx := gorums.TestContext(t, 2*time.Second)
	err := gorums.Multicast(
		cfgClient.Context(ctx),
		pb.String("trigger"),
		mock.TestMethod,
	)
	if err != nil {
		t.Fatalf("multicast error: %v", err)
	}

	waitWithTimeout(t, &wg)
}
