package conn

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// startEchoServer starts a gRPC server whose node streams echo every message,
// and returns its address. The server stops when the test ends.
func startEchoServer(tb testing.TB) string {
	tb.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("failed to listen: %v", err)
	}
	grpcSrv := grpc.NewServer() // skipcq: GO-S0902
	stream.RegisterGorumsServer(grpcSrv, benchEchoServer{})
	go func() { _ = grpcSrv.Serve(lis) }()
	tb.Cleanup(grpcSrv.Stop)
	return lis.Addr().String()
}

// TestOutboundManagerPeerVerifier verifies that a configuration carrying a
// server checks each connection to a configured peer with the server's peer
// verifier, and leaves other nodes unchecked. A peer verifier requires
// transport credentials, because the check runs after their handshake.
func TestOutboundManagerPeerVerifier(t *testing.T) {
	addr := startEchoServer(t)
	errRefused := errors.New("certificate names another node")
	tests := []struct {
		name          string
		nodeID        ID // 2 is a configured peer; 5 is not
		noVerifier    bool
		noCredentials bool
		verifyErr     error
		wantCode      codes.Code
		wantChecked   []ID
		wantNodeErr   string
	}{
		{name: "PeerAccepted", nodeID: 2, wantCode: codes.OK, wantChecked: []ID{2}},
		{name: "PeerRefused", nodeID: 2, verifyErr: errRefused, wantCode: codes.Unauthenticated, wantChecked: []ID{2}},
		{name: "NotAPeer", nodeID: 5, wantCode: codes.OK},
		{name: "NoVerifier", nodeID: 2, noVerifier: true, wantCode: codes.OK},
		{name: "NoVerifierNoCredentials", nodeID: 2, noVerifier: true, noCredentials: true, wantCode: codes.OK},
		{name: "NoCredentials", nodeID: 2, noCredentials: true, wantNodeErr: "gorums: node 2 has a TLS identity; set using WithTransportCredentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var checked []ID
			var verify PeerVerifier
			if !tt.noVerifier {
				verify = func(_ context.Context, id ID) error {
					mu.Lock()
					defer mu.Unlock()
					checked = append(checked, id)
					return tt.verifyErr
				}
			}
			im, err := NewInboundManager(1, WithNodes(map[ID]mock.NodeAddr{
				1: "127.0.0.1:9081",
				2: mock.NodeAddr(addr),
			}), 0, DefaultRecvBufferSize, nil, nil, verify)
			if err != nil {
				t.Fatalf("NewInboundManager: %v", err)
			}
			m := newOutboundManager(func(o *DialOptions) {
				o.InboundManager = im
				o.LocalNodeID = 1
				o.GRPCDialOptions = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
				if !tt.noCredentials {
					o.Credentials = insecure.NewCredentials()
				}
			})
			t.Cleanup(m.Close)
			n, err := m.newNode(tt.nodeID, addr, nil)
			if tt.wantNodeErr != "" {
				if err == nil || err.Error() != tt.wantNodeErr {
					t.Fatalf("newNode(%d) error = %v; want %q", tt.nodeID, err, tt.wantNodeErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newNode(%d): %v", tt.nodeID, err)
			}

			responseChan := make(chan stream.NodeResponse[*stream.Message], 1)
			msg := stream.Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build()
			NodeTransport(n).Enqueue(stream.Request{Ctx: t.Context(), Msg: msg, Oneway: true, ResponseChan: responseChan})
			select {
			case r := <-responseChan:
				if got := status.Code(r.Err); got != tt.wantCode {
					t.Errorf("send error = %v; want code %v", r.Err, tt.wantCode)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no send result within 3s")
			}

			mu.Lock()
			defer mu.Unlock()
			// The node checks each connection it makes, and it may make more
			// than one, so compare the distinct IDs.
			if got := slices.Compact(checked); !slices.Equal(got, tt.wantChecked) {
				t.Errorf("verifier checked %v; want %v", got, tt.wantChecked)
			}
		})
	}
}
