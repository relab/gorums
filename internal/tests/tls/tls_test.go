package tls

import (
	context "context"
	"testing"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/gorumstest/certs"
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
	cert := certs.Loopback(t)
	tlsCert := cert.TLS()

	srvFn := func(_ int) gorumstest.ServerIface {
		srv := gorums.NewServer(gorums.WithGRPCServerOptions(grpc.Creds(credentials.NewServerTLSFromCert(&tlsCert))))
		RegisterTLSServer(srv, &testSrv{})
		return srv
	}
	creds := credentials.NewClientTLSFromCert(cert.Pool(), "")
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
