package tls

import (
	context "context"
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type testSrv struct{}

func (t testSrv) TestTLS(ctx gorums.ServerCtx, in *Request) (resp *Response, err error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok || peerInfo.AuthInfo.AuthType() != "tls" {
		return &Response{OK: false}, nil
	}
	return &Response{OK: true}, nil
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

	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		srv := NewServer(gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewServerTLSFromCert(&tlsCert))))
		RegisterTLSServer(srv, &testSrv{})
		return srv
	})
	defer teardown()

	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")),
		),
	)
	_, err = mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
	resp, err := node.TestTLS(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("RPC error: %v", err)
	}

	if !resp.GetOK() {
		t.Fatalf("TestTLS() == %t, want %t", resp.GetOK(), true)
	}
}
