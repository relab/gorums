package tls

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mockcall"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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
	opts = append([]gorumstest.Option{
		gorums.WithLocalNodes(func(id gorums.ID, addr string) gorums.NodeAddress {
			return peerNode{addr, nodeIdentity(id)}
		}),
		gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
			return []gorums.ServerOption{
				gorums.WithGRPCServerOptions(grpc.Creds(ca.serverCreds(certs[id]))),
			}
		}),
		gorums.WithLocalDialOptionsFunc(func(id gorums.ID) []gorums.DialOption {
			return []gorums.DialOption{gorums.WithTransportCredentials(ca.dialCreds(certs[id]))}
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
	resp, err := mockcall.RemoteCall(ctx, node, "hello")
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

// TestPeerIdentityDialCheckOpensNoStream verifies that a dialer checks the
// server's identity in the TLS handshake: a server that presents another
// node's certificate never receives a stream from a peer that dials it, and
// that peer paces its handshakes while its calls fail.
func TestPeerIdentityDialCheckOpensNoStream(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			ca := newTestCA(t)
			var streams, handshakes atomic.Int32
			// Server 2 serves with node 3's certificate, and counts the
			// handshakes and streams from server 1.
			observeServer2 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
				if id != 2 {
					return nil
				}
				cfg := &tls.Config{
					Certificates: []tls.Certificate{ca.issue(t, 3)},
					ClientCAs:    ca.pool,
					ClientAuth:   tls.VerifyClientCertIfGiven,
					VerifyConnection: func(cs tls.ConnectionState) error {
						if len(cs.PeerCertificates) > 0 && slices.ContainsFunc(cs.PeerCertificates[0].URIs, func(u *url.URL) bool {
							return u.String() == nodeIdentity(1)
						}) {
							handshakes.Add(1)
						}
						return nil
					},
				}
				return []gorums.ServerOption{gorums.WithGRPCServerOptions(
					grpc.Creds(credentials.NewTLS(cfg)),
					grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						md, _ := metadata.FromIncomingContext(stream.Context())
						if slices.Contains(md.Get("gorums-node-id"), "1") {
							streams.Add(1)
						}
						return handler(srv, stream)
					}),
				)}
			})
			servers, _ := peerGroup(t, ca, 3, nil, append(mode.opts, observeServer2)...)
			node2 := servers[0].PeerConfig().Node(2)
			err := awaitCode(t, node2, codes.Unauthenticated)
			if want := "gorums: node 2 failed verification"; !strings.Contains(err.Error(), want) {
				t.Errorf("call from server 1 to node 2: err = %q; want it to contain %q", err, want)
			}

			before := handshakes.Load()
			const window = time.Second
			calls := 0
			for deadline := time.Now().Add(window); time.Now().Before(deadline); calls++ {
				if _, err := remoteCall(t, node2, 3*time.Second); status.Code(err) != codes.Unauthenticated {
					t.Fatalf("call %d from server 1 to node 2: err = %v; want code %v", calls+1, err, codes.Unauthenticated)
				}
			}
			t.Logf("%d refused calls, %d handshakes on server 2", calls, handshakes.Load()-before)
			if got := streams.Load(); got != 0 {
				t.Errorf("server 2 received %d streams from server 1; want 0", got)
			}
			// gRPC's reconnect backoff starts at 1 s.
			if got := handshakes.Load() - before; got > 3 {
				t.Errorf("server 2 saw %d handshakes from server 1 in %v; want at most 3", got, window)
			}
		})
	}
}

// TestPeerIdentityRefusalIsPaced verifies that a refused peer's calls fail
// with the refusal at once, and that the refused peer paces its new streams,
// so the refusing server receives only a few streams.
func TestPeerIdentityRefusalIsPaced(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			var streams atomic.Int32
			countOnServer3 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
				if id != 3 {
					return nil
				}
				return []gorums.ServerOption{
					gorums.WithGRPCServerOptions(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						streams.Add(1)
						return handler(srv, stream)
					})),
				}
			})
			// Server 2 presents node 3's certificate, so server 3 refuses
			// server 2's claim.
			servers, _ := peerGroup(t, newTestCA(t), 3, []gorums.ID{1, 3, 3}, append(mode.opts, countOnServer3)...)
			node3 := servers[1].PeerConfig().Node(3)
			awaitCode(t, node3, codes.Unauthenticated)

			before := streams.Load()
			const window = time.Second
			calls := 0
			for deadline := time.Now().Add(window); time.Now().Before(deadline); calls++ {
				if _, err := remoteCall(t, node3, 3*time.Second); status.Code(err) != codes.Unauthenticated {
					t.Fatalf("call %d from server 2 to node 3: err = %v; want code %v", calls+1, err, codes.Unauthenticated)
				}
			}
			t.Logf("%d refused calls, %d streams on server 3", calls, streams.Load()-before)
			if got := streams.Load() - before; got > 20 {
				t.Errorf("server 3 received %d streams during %d refused calls in %v; want at most 20", got, calls, window)
			}
		})
	}
}

// TestPeerIdentityRefusedPeerNeverConnected verifies that a server whose
// streams a peer refuses never counts that peer as connected: its peer-change
// callback never reports the peer, though it keeps opening new streams.
func TestPeerIdentityRefusedPeerNeverConnected(t *testing.T) {
	for _, mode := range peerIdentityModes {
		t.Run(mode.name, func(t *testing.T) {
			var reported atomic.Bool
			watchServer2 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
				if id != 2 {
					return nil
				}
				return []gorums.ServerOption{gorums.WithPeerChange(func(cfg gorums.Config) {
					if slices.Contains(cfg.NodeIDs(), 3) {
						reported.Store(true)
					}
				})}
			})
			// Server 2 presents node 3's certificate, so server 3 refuses
			// server 2's streams.
			servers, _ := peerGroup(t, newTestCA(t), 3, []gorums.ID{1, 3, 3}, append(mode.opts, watchServer2)...)
			node3 := servers[1].PeerConfig().Node(3)
			awaitLastErrCode(t, node3, codes.Unauthenticated)
			// Calls open new streams, which server 3 refuses too.
			for deadline := time.Now().Add(500 * time.Millisecond); time.Now().Before(deadline); {
				if _, err := remoteCall(t, node3, 3*time.Second); status.Code(err) != codes.Unauthenticated {
					t.Fatalf("call from server 2 to node 3: err = %v; want code %v", err, codes.Unauthenticated)
				}
			}
			if reported.Load() {
				t.Error("server 2 reported node 3 as connected; want it never connected")
			}
			if slices.Contains(servers[1].ConnectedPeers().NodeIDs(), 3) {
				t.Error("server 2's connected peers include node 3; want it never connected")
			}
		})
	}
}

// TestPeerIdentityReadyAfterRefusal verifies optimistic readiness for idle
// accepted streams, even when GOAWAY keeps their lifetimes below two seconds.
func TestPeerIdentityReadyAfterRefusal(t *testing.T) {
	var allow atomic.Bool
	var accepted atomic.Int32
	refuseOn2 := gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
		if id != 2 {
			return nil
		}
		return []gorums.ServerOption{
			gorums.WithGRPCServerOptions(grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionAge:      300 * time.Millisecond,
				MaxConnectionAgeGrace: 50 * time.Millisecond,
			})),
			gorums.WithGRPCServerOptions(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				if !allow.Load() {
					return status.Error(codes.Unauthenticated, "peer is not trusted")
				}
				accepted.Add(1)
				return handler(srv, stream)
			})),
		}
	})
	servers, calls := peerGroup(t, newTestCA(t), 2, nil, gorums.WithStreamDedup(), refuseOn2)
	observer := servers[0]
	awaitLastErrCode(t, observer.PeerConfig().Node(2), codes.Unauthenticated)
	allow.Store(true)

	// No application frames arrive to establish readiness. Each new
	// accepted stream must count as connected without a probation timer.
	for cycle := int32(1); cycle <= 3; cycle++ {
		if !gorumstest.WaitUntil(t, 3*time.Second, func() bool { return accepted.Load() >= cycle }) {
			t.Fatalf("accepted %d streams; want at least %d", accepted.Load(), cycle)
		}
		if _, err := observer.WaitForAll(gorumstest.Context(t, time.Second)); err != nil {
			t.Fatalf("idle stream %d: WaitForAll = %v (accepted %d streams)", cycle, err, accepted.Load())
		}
	}
	if got := calls[0].Load() + calls[1].Load(); got != 0 {
		t.Errorf("handled %d requests; want idle streams", got)
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
