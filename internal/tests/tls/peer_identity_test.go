package tls

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// peerGroup starts n servers that check each other's identities with
// per-replica certificates from ca. Server id presents the certificate of
// node certIDs[id-1], or its own if certIDs is nil, both to clients that dial
// it and to the peers it dials. Each server answers a mock.TestMethod
// request with "server <id> from <sender ID>: <request>", so a reply also
// shows the [gorums.ServerContext.SenderID] the handler saw, and counts the
// requests in calls[id-1].
func peerGroup(t *testing.T, ca *testCA, n int, certIDs []gorums.ID, opts ...gorumstest.Option) (servers []*gorums.Server, calls []*atomic.Int32) {
	t.Helper()
	certs := make(map[gorums.ID]tls.Certificate, n)
	for id := gorums.ID(1); id <= gorums.ID(n); id++ {
		certID := id
		if certIDs != nil {
			certID = certIDs[id-1]
		}
		certs[id] = ca.issue(t, certID)
	}
	verify := peerVerifier(n)
	opts = append([]gorumstest.Option{
		gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
			return []gorums.ServerOption{
				gorums.WithGRPCServerOptions(grpc.Creds(ca.serverCreds(certs[id]))),
				gorums.WithPeerVerifier(verify),
			}
		}),
		gorums.WithLocalDialOptionsFunc(func(id gorums.ID) []gorums.DialOption {
			return []gorums.DialOption{gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(ca.dialCreds(certs[id])))}
		}),
	}, opts...)
	servers = gorumstest.LocalServers(t, n, opts...)
	for _, srv := range servers {
		count := new(atomic.Int32)
		calls = append(calls, count)
		self := srv.NodeID()
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			count.Add(1)
			req := gorums.AsProto[*pb.StringValue](in)
			reply := fmt.Sprintf("server %d from %d: %s", self, ctx.SenderID(), req.GetValue())
			return gorums.NewResponseMessage(in, pb.String(reply)), nil
		})
	}
	return servers, calls
}

// remoteCall calls mock.TestMethod on node and returns the reply's value.
func remoteCall(t *testing.T, node *gorums.Node, timeout time.Duration) (string, error) {
	t.Helper()
	ctx := gorumstest.Context(t, timeout)
	resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(ctx), pb.String("hello"), mock.TestMethod)
	return resp.GetValue(), err
}

// awaitCode calls node until the call fails with code, and fails the test if
// that does not happen within 5 seconds. Until the connection is ready, a
// call can fail for that reason instead.
func awaitCode(t *testing.T, node *gorums.Node, code codes.Code) error {
	t.Helper()
	var err error
	if !gorumstest.WaitUntil(t, 5*time.Second, func() bool {
		_, err = remoteCall(t, node, time.Second)
		return status.Code(err) == code
	}) {
		t.Fatalf("call to node %d: err = %v; want code %v", node.ID(), err, code)
	}
	return err
}

// awaitLastErrCode fails the test unless node's last error has code within 3 seconds.
func awaitLastErrCode(t *testing.T, node *gorums.Node, code codes.Code) {
	t.Helper()
	if !gorumstest.WaitUntil(t, 3*time.Second, func() bool {
		return status.Code(node.LastErr()) == code
	}) {
		t.Errorf("node %d LastErr = %v; want code %v", node.ID(), node.LastErr(), code)
	}
}

var peerIdentityModes = []struct {
	name string
	opts []gorumstest.Option
}{
	{name: "Dual"},
	{name: "Dedup", opts: []gorumstest.Option{gorums.WithStreamDedup()}},
}

// TestPeerIdentityConnects verifies that peers with their own certificates
// connect, and that each handler sees the authenticated ID of the server that
// called it.
func TestPeerIdentityConnects(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			servers, _ := peerGroup(t, newTestCA(t), 3, nil, mode.opts...)
			gorumstest.WaitForPeers(t, servers)
			for _, srv := range servers {
				for _, node := range srv.PeerConfig() {
					got, err := remoteCall(t, node, 3*time.Second)
					if err != nil {
						t.Fatalf("server %d: call to node %d: %v", srv.NodeID(), node.ID(), err)
					}
					// The handler on node.ID() sees the calling server as the sender.
					if want := fmt.Sprintf("server %d from %d: hello", node.ID(), srv.NodeID()); got != want {
						t.Errorf("server %d: call to node %d = %q; want %q", srv.NodeID(), node.ID(), got, want)
					}
				}
			}
		})
	}
}

// TestPeerIdentityWrongCertificate verifies both checks with a server that
// presents another node's certificate: a peer that dials it refuses it, and
// a peer it dials refuses its claim.
func TestPeerIdentityWrongCertificate(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			// Server 2 presents node 3's certificate.
			servers, _ := peerGroup(t, newTestCA(t), 3, []gorums.ID{1, 3, 3}, mode.opts...)

			// Dial-side check: server 1 dials server 2 and finds node 3's certificate.
			err := awaitCode(t, servers[0].PeerConfig().Node(2), codes.Unauthenticated)
			if want := "gorums: node 2 failed verification"; !strings.Contains(err.Error(), want) {
				t.Errorf("call from server 1 to node 2: err = %q; want it to contain %q", err, want)
			}
			// Inbound check: server 2 dials server 3 and claims ID 2 with
			// node 3's certificate; its calls fail with server 3's refusal.
			awaitCode(t, servers[1].PeerConfig().Node(3), codes.Unauthenticated)
			awaitLastErrCode(t, servers[1].PeerConfig().Node(3), codes.Unauthenticated)
			// The other pair is unaffected.
			if got, err := remoteCall(t, servers[0].PeerConfig().Node(3), 3*time.Second); err != nil || got != "server 3 from 1: hello" {
				t.Errorf("call from server 1 to node 3 = %q, %v; want \"server 3 from 1: hello\", nil", got, err)
			}
		})
	}
}

// TestPeerIdentityRefusalIsPaced verifies that a refused peer's calls fail
// with the refusal at once, and that the refused peer paces its new streams,
// so the refusing server runs its verifier only a few times.
func TestPeerIdentityRefusalIsPaced(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			var checks atomic.Int32
			verify := peerVerifier(3)
			countOnServer3 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
				if id != 3 {
					return nil
				}
				return []gorums.ServerOption{gorums.WithPeerVerifier(func(ctx context.Context, id gorums.ID) error {
					checks.Add(1)
					return verify(ctx, id)
				})}
			})
			// Server 2 presents node 3's certificate, so server 3 refuses
			// server 2's claim.
			servers, _ := peerGroup(t, newTestCA(t), 3, []gorums.ID{1, 3, 3}, append(mode.opts, countOnServer3)...)
			node3 := servers[1].PeerConfig().Node(3)
			awaitCode(t, node3, codes.Unauthenticated)

			before := checks.Load()
			const window = time.Second
			calls := 0
			for deadline := time.Now().Add(window); time.Now().Before(deadline); calls++ {
				if _, err := remoteCall(t, node3, 3*time.Second); status.Code(err) != codes.Unauthenticated {
					t.Fatalf("call %d from server 2 to node 3: err = %v; want code %v", calls+1, err, codes.Unauthenticated)
				}
			}
			t.Logf("%d refused calls, %d verifier runs on server 3", calls, checks.Load()-before)
			if got := checks.Load() - before; got > 20 {
				t.Errorf("server 3 ran its verifier %d times during %d refused calls in %v; want at most 20", got, calls, window)
			}
		})
	}
}

// TestPeerIdentityRefusedPeerNotConnected verifies that, under stream
// deduplication, a server whose stream to one peer is refused does not count
// that peer as connected, so its WaitForAll does not return.
func TestPeerIdentityRefusedPeerNotConnected(t *testing.T) {
	verify := peerVerifier(3)
	// Server 3 refuses server 2's claim only. Its check takes a while, as a
	// real check may, so each refused stream would count as up meanwhile.
	refuse2On3 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
		if id != 3 {
			return nil
		}
		return []gorums.ServerOption{gorums.WithPeerVerifier(func(ctx context.Context, id gorums.ID) error {
			if id == 2 {
				time.Sleep(20 * time.Millisecond)
				return errors.New("server 2 is not trusted")
			}
			return verify(ctx, id)
		})}
	})
	servers, _ := peerGroup(t, newTestCA(t), 3, nil, gorums.WithStreamDedup(), refuse2On3)
	server2 := servers[1]
	awaitCode(t, server2.PeerConfig().Node(3), codes.Unauthenticated)
	// The first stream counted as up until its refusal arrived.
	if !gorumstest.WaitUntil(t, 3*time.Second, func() bool {
		return !slices.Contains(server2.ConnectedPeers().NodeIDs(), 3)
	}) {
		t.Fatalf("server 2 ConnectedPeers = %v; want it without node 3", server2.ConnectedPeers().NodeIDs())
	}

	// Later streams, which server 3 refuses too, never count as up.
	ctx := gorumstest.Context(t, 500*time.Millisecond)
	if _, err := server2.WaitForAll(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("server 2 WaitForAll = %v; want %v, since server 3 refuses server 2", err, context.DeadlineExceeded)
	}
}

// TestPeerIdentityClients verifies how a server with peer identity checks
// treats clients: a client that claims a peer's ID without that peer's
// certificate is refused and cannot take over the peer's stream, and a
// client that claims no ID is served without a certificate.
func TestPeerIdentityClients(t *testing.T) {
	ca := newTestCA(t)
	// Under stream deduplication, server 2 sends its calls to node 1 over
	// the stream that server 1 opened, which a forged claim would replace.
	servers, calls := peerGroup(t, ca, 3, nil, gorums.WithStreamDedup())
	gorumstest.WaitForPeers(t, servers)
	server2 := servers[1]
	node3Cert := ca.issue(t, 3)
	claim1 := gorums.WithMetadata(metadata.Pairs("gorums-node-id", "1"))
	// The in-memory dialer belongs to the test that started the servers, so
	// the subtests dial with this test's dial options.
	dialOpt := gorumstest.DialOptions(t)

	tests := []struct {
		name      string
		certs     []tls.Certificate
		opts      []gorums.DialOption
		wantServe bool
	}{
		{name: "ImpostorClaimsLowerPeer", certs: []tls.Certificate{node3Cert}, opts: []gorums.DialOption{claim1}},
		{name: "ClaimWithoutCertificate", opts: []gorums.DialOption{claim1}},
		{name: "ClientWithoutCertificate", wantServe: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := calls[1].Load()
			opts := append([]gorums.DialOption{
				dialOpt,
				gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(ca.dialCreds(tt.certs...))),
			}, tt.opts...)
			cfg, closeFn, err := gorums.NewConfig(gorums.WithNodeList([]string{server2.Addr()}), opts...)
			if err != nil {
				t.Fatalf("NewConfig: %v", err)
			}
			t.Cleanup(closeFn)
			node := cfg[0]

			if tt.wantServe {
				// A client that announces no ID has sender ID 0.
				if got, err := remoteCall(t, node, 3*time.Second); err != nil || got != "server 2 from 0: hello" {
					t.Errorf("call from the client = %q, %v; want \"server 2 from 0: hello\", nil", got, err)
				}
				return
			}
			// The call fails with the server's refusal, which is also the
			// node's last error.
			if _, err := remoteCall(t, node, 3*time.Second); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("call from a refused client: err = %v; want code %v", err, codes.Unauthenticated)
			}
			awaitLastErrCode(t, node, codes.Unauthenticated)
			if n := calls[1].Load() - before; n != 0 {
				t.Errorf("server 2 handled %d requests from a refused client; want 0", n)
			}
			// Server 2's calls to node 1 still reach server 1.
			if got, err := remoteCall(t, server2.PeerConfig().Node(1), 3*time.Second); err != nil || got != "server 1 from 2: hello" {
				t.Errorf("call from server 2 to node 1 = %q, %v; want \"server 1 from 2: hello\", nil", got, err)
			}
		})
	}
}
