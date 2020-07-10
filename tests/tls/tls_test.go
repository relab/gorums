package tls

import (
	context "context"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type testSrv struct{}

func (t testSrv) TestTLS(ctx context.Context, in *Request, out func(*Response, error)) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok || peerInfo.AuthInfo.AuthType() != "tls" {
		out(&Response{OK: false}, nil)
		return
	}
	out(&Response{OK: true}, nil)
}

func TestTLS(t *testing.T) {
	cert, key, err := generateCert()
	if err != nil {
		t.Errorf("Failed to generate certificate: %v", err)
	}

	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(cert) {
		t.Errorf("Failed to add cert to pool.")
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Errorf("Failed to parse cert: %v", err)
	}

	addrs, teardown := gorums.TestSetup(t, 1, func() interface{} {
		srv := NewGorumsServer(WithGRPCServerOptions(grpc.Creds(credentials.NewServerTLSFromCert(&tlsCert))))
		srv.RegisterTLSServer(&testSrv{})
		return srv
	})
	defer teardown()

	mgr, err := NewManager(WithNodeList(addrs), WithDialTimeout(100*time.Millisecond), WithGrpcDialOptions(
		grpc.WithBlock(),
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")),
		grpc.WithReturnConnectionError(),
	))
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	node := mgr.Nodes()[0]

	resp, err := node.TestTLS(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Failed to perform RPC: %v", err)
	}

	if !resp.GetOK() {
		t.Fatalf("TLS was not enabled")
	}
}
