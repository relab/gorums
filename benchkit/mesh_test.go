package benchkit

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// streamModes lists the two stream topologies that AwaitMesh must support.
var streamModes = []struct {
	name string
	opts []gorums.ServerOption
}{
	{name: "Dual"},
	{name: "Dedup", opts: []gorums.ServerOption{gorums.WithStreamDedup()}},
}

// meshServers returns n unstarted servers that form one peer group, with the
// options from fn applied to each server by node ID. The listeners are bound
// once and stay open until the test ends. The test registers its services and
// then serves.
func meshServers(t *testing.T, n int, opts []gorums.ServerOption, fn func(id gorums.ID) []gorums.ServerOption) []*gorums.Server {
	t.Helper()
	localOpts := []gorums.LocalServerOption{
		gorums.WithLocalServerOptions(opts...),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	}
	if fn != nil {
		localOpts = append(localOpts, gorums.WithLocalServerOptionsFunc(fn))
	}
	servers, stop, err := gorums.NewLocalServers(n, localOpts...)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)
	return servers
}

// serve starts serving each of servers.
func serve(servers ...*gorums.Server) {
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
}

// streamGate returns a server option that refuses every stream from the peer
// with node ID from while the gate is closed. The gate starts closed; the
// returned function opens it.
func streamGate(from gorums.ID) (gorums.ServerOption, func()) {
	var open atomic.Bool
	sender := []string{strconv.FormatUint(uint64(from), 10)}
	opt := gorums.WithGRPCServerOptions(grpc.StreamInterceptor(
		func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			md, _ := metadata.FromIncomingContext(ss.Context())
			if !open.Load() && slices.Equal(md.Get("gorums-node-id"), sender) {
				return status.Error(codes.Unavailable, "stream held back by test")
			}
			return handler(srv, ss)
		}))
	return opt, func() { open.Store(true) }
}

// TestMeshAwaitMeshWaitsForPeerView verifies that AwaitMesh passes only once
// every peer reports a complete view. Node 3 refuses the streams of node 2, so
// node 1 can reach both peers while node 2 has no stream to node 3. AwaitMesh
// must fail and name the missing connection, pass once node 3 accepts node 2,
// and fail again from fresh views after node 3 stops.
func TestMeshAwaitMeshWaitsForPeerView(t *testing.T) {
	for _, mode := range streamModes {
		t.Run(mode.name, func(t *testing.T) {
			gate, openGate := streamGate(2)
			servers := meshServers(t, 3, mode.opts, func(id gorums.ID) []gorums.ServerOption {
				if id == 3 {
					return []gorums.ServerOption{gate}
				}
				return nil
			})
			for _, srv := range servers {
				RegisterControlServer(srv, NewControl())
			}
			serve(servers...)
			node1 := servers[0]

			// Node 1 reaches both peers.
			full := func(cfg gorums.Config) bool { return cfg.Size() == 3 }
			if err := node1.WaitForPeers(gorumstest.Context(t, 5*time.Second), full); err != nil {
				t.Fatalf("node 1 WaitForPeers: %v", err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			err := AwaitMesh(ctx, node1.PeerConfig())
			cancel()
			if err == nil {
				t.Fatal("AwaitMesh = nil while node 2 has no stream to node 3, want error")
			}
			missing := "node 2 (" + servers[1].Addr() + ") is missing peers [3]"
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("AwaitMesh error = %q, want it to contain %q", err, missing)
			}
			if strings.Contains(err.Error(), "did not answer") {
				t.Errorf("AwaitMesh error = %q, want every node to answer", err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("AwaitMesh error = %v, want it to wrap %v", err, context.DeadlineExceeded)
			}

			openGate()
			if err := AwaitMesh(gorumstest.Context(t, 10*time.Second), node1.PeerConfig()); err != nil {
				t.Fatalf("AwaitMesh after opening the gate: %v", err)
			}

			// Node 2's view loses node 3. A fresh probe must report it.
			servers[2].Stop()
			lost := func(cfg gorums.Config) bool { return !cfg.Contains(3) }
			if err := servers[1].WaitForPeers(gorumstest.Context(t, 5*time.Second), lost); err != nil {
				t.Fatalf("node 2 WaitForPeers(lost node 3): %v", err)
			}
			ctx, cancel = context.WithTimeout(t.Context(), time.Second)
			err = AwaitMesh(ctx, node1.PeerConfig())
			cancel()
			if err == nil {
				t.Fatal("AwaitMesh = nil after node 3 stopped, want error")
			}
			for _, want := range []string{missing, "node 3 (" + servers[2].Addr() + ") did not answer"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("AwaitMesh error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

// TestMeshAwaitMeshStalledPeer verifies that a peer that never answers makes
// AwaitMesh return within the bound of its context, with an error that names
// the peer and wraps the context error.
func TestMeshAwaitMeshStalledPeer(t *testing.T) {
	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		wantErr error
	}{
		{
			name: "Deadline",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "Cancel",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				time.AfterFunc(500*time.Millisecond, cancel)
				return ctx
			},
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := meshServers(t, 3, nil, nil)
			for _, srv := range servers[:2] {
				RegisterControlServer(srv, NewControl())
			}
			// Node 3 receives PeerView calls but never replies.
			servers[2].RegisterHandler("benchkit.Control.PeerView", func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
				return nil
			})
			serve(servers...)

			start := time.Now()
			err := AwaitMesh(tt.ctx(t), servers[0].PeerConfig())
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Errorf("AwaitMesh returned after %v, want it bounded by its context", elapsed)
			}
			if err == nil {
				t.Fatal("AwaitMesh = nil with a stalled peer, want error")
			}
			if want := "node 3 (" + servers[2].Addr() + ") did not answer"; !strings.Contains(err.Error(), want) {
				t.Errorf("AwaitMesh error = %q, want it to contain %q", err, want)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("AwaitMesh error = %v, want it to wrap %v", err, tt.wantErr)
			}
		})
	}
}
