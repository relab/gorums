package tls

import (
	context "context"
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type testSrv struct{}

func (testSrv) TestTLS(ctx gorums.ServerContext, _ *Request) (resp *Response, err error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok || peerInfo.AuthInfo.AuthType() != "tls" {
		return Response_builder{Valid: false}.Build(), nil
	}
	return Response_builder{Valid: true}.Build(), nil
}

func TestTLSConnection(t *testing.T) {
	cert, key, err := generateCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(cert) {
		t.Fatalf("Failed to add cert to pool.")
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Fatalf("Failed to parse cert: %v", err)
	}

	srvFn := func(_ int) gorumstest.ServerIface {
		srv := gorums.NewServer(gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewServerTLSFromCert(&tlsCert))))
		RegisterTLSServer(srv, &testSrv{})
		return srv
	}
	creds := credentials.NewClientTLSFromCert(cp, "")
	tests := []struct {
		name string
		opt  gorums.DialOption
	}{
		{name: "GRPCDialOptions", opt: gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(creds))},
		{name: "TransportCredentials", opt: gorums.WithTransportCredentials(creds)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := gorumstest.Node(t, srvFn, tt.opt)
			resp, err := NewTLSNodeClient(node).TestTLS(context.Background(), &Request{})
			if err != nil {
				t.Fatalf("RPC error: %v", err)
			}
			if !resp.GetValid() {
				t.Fatalf("TestTLS() == %t, want %t", resp.GetValid(), true)
			}
		})
	}
}
