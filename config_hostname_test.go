package gorums_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// selfSignedCert returns a certificate valid only for the DNS name "localhost"
// (no IP SANs) and a pool that trusts it.
func selfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// TestConfigHostnameTLS verifies that a node configured with a hostname dials
// the hostname, so TLS verifies a DNS-only certificate against it.
func TestConfigHostnameTLS(t *testing.T) {
	cert, pool := selfSignedCert(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := gorums.NewServer(gorums.WithGRPCServerOptions(
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
	))
	srv.RegisterHandler(mock.TestMethod, gorumstest.EchoHandler("echo"))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	addr := fmt.Sprintf("localhost:%s", port)
	cfg, closeFn, err := gorums.NewConfig(
		gorums.WithNodeList([]string{addr}),
		gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: pool}))),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)

	node := cfg[0]
	if got := node.Address(); got != addr {
		t.Errorf("Address() = %q, want %q", got, addr)
	}
	ctx := gorumstest.Context(t, 5*time.Second)
	resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(ctx), pb.String("x"), mock.TestMethod)
	if err != nil {
		t.Fatalf("RemoteCall over TLS to %s: %v", addr, err)
	}
	if got, want := resp.GetValue(), "echo: x"; got != want {
		t.Errorf("response = %q, want %q", got, want)
	}
}
