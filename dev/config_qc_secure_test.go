package dev_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"
	"github.com/relab/gorums/internal/leakcheck"
	"google.golang.org/grpc/credentials"
)

// To generate new keys and certificates you can use the certstrap
// tool as shown below. You can get the tool through homebrew:
//
// % brew install certstrap
// OR
// % git clone https://github.com/square/certstrap
//
// The current key and certificate used in the tests have been
// copied from files 'out/127.0.0.1.crt' and 'out/127.0.0.1.key'
// after running the following commands:
//
// % certstrap init --common-name "gorums.org"
// % certstrap request-cert -ip 127.0.0.1
// % certstrap sign 127.0.0.1 --CA gorums.org
//
// For additional documentation regarding setting up secure gRPC, see
// https://bbengfort.github.io/programmer/2017/03/03/secure-grpc.html

var (
	serverCRT = `-----BEGIN CERTIFICATE-----
MIIEODCCAiCgAwIBAgIRAOVXpXvHjS/0TzD8VYkpnxIwDQYJKoZIhvcNAQELBQAw
FTETMBEGA1UEAxMKZ29ydW1zLm9yZzAeFw0xNzA5MTIyMjI0NDBaFw0xOTA5MTIy
MjI0NDBaMBQxEjAQBgNVBAMTCTEyNy4wLjAuMTCCASIwDQYJKoZIhvcNAQEBBQAD
ggEPADCCAQoCggEBAJvtFrfsu5bHOK7TkTHymHBClnSZKqlnlZkrrizOjGYDPr1w
t1MeEoKnjBQfSiuSsIzhpIfsza7EoQqpV3hWmLSg7RkKLyeGG9b1pbbyaLZDyljt
3ozLFfPh1oubwQtVAdVqJfIScfTv8tm/KUNg2nkMxshesG/gOm/BEKOdioSASmMS
SD6Xqz187TxTHyq4jSK1zR7E2D1plxyEa+xf1wtZqKBHVz5DiWWSVOnWuqdI+VFy
mEZgDzD3an+2KN/HFF3zq1ciRwp/fh7I0B1MjrbNEP5dgKrVtI8tSSIU731VUm8W
0EmnDToNKx2L0cL3l9I1pjKVI2jTXYZkmhSVMskCAwEAAaOBgzCBgDAOBgNVHQ8B
Af8EBAMCA7gwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMB0GA1UdDgQW
BBQ0DvmfxNXQ7L3tRtCkM70VeXYmPDAfBgNVHSMEGDAWgBQZCLlzr+Uc3rdSCUR4
9pBIcoHe3jAPBgNVHREECDAGhwR/AAABMA0GCSqGSIb3DQEBCwUAA4ICAQCg8ybL
5wTLZkmTHaAHX4cXAEYIKdr1hupAJrjLh8fKY7EzSI0b6CCgBX9jzEz76WcHCTJD
jY3fm+uYBbsB6ToCzqjv0bPtD3RyEVj7oDu+YsXIWLWeMDEGeIUy2qylNqKVJbKS
xaghlg60simzJtR2mQm/SmAIfcU+8kDyAYv1l2p+mvzHgryQPTybh6xKGeSpoC1o
JxtR1LhIPHih0C3FgC2W/nUAKKe/Lv6Zt+EDGJ/Yk9X80gP4v5RPytphflMudodn
Vvr0ZjBAc+blhRwTMVzxHJoJzwuCE8rYZWxjI+3cFNzGxls8IdphHrOepSlqXXHd
0rBdHBwCg4Nr/AYR6/HRG73E9dDGWw8WqRu2bOyqPognPxD/m9J24ocAumfT4e/8
7Weo2Z+5uJ2NUgERUl3SzEBWMO7s1NuqUkn7Tnmi5DAGiZr/Ykrot/mLUCsxRsOR
HPreqRWk7bSDtHCmKUnGUj3t6Uw6+GFEfuMwl4n4Dr9XGRmLl8tAyWU/Q87MU+Yy
Qj3E91GCrKqWgqpspSepz/nOU3ZkaMPYcp98ZAehYWBF5Dd/Y/r5Oo8HwnwHta3y
Hi+MnWeLAwM+yGxnIftBDm2PDMpfNksVbOaCFZit1GEG7dz0bSTZh9oJtSXucAKs
xcnHJWVF35BQqMaNKccUnxUM7x8ujle/GgB7Hw==
-----END CERTIFICATE-----`

	serverKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAm+0Wt+y7lsc4rtORMfKYcEKWdJkqqWeVmSuuLM6MZgM+vXC3
Ux4SgqeMFB9KK5KwjOGkh+zNrsShCqlXeFaYtKDtGQovJ4Yb1vWltvJotkPKWO3e
jMsV8+HWi5vBC1UB1Wol8hJx9O/y2b8pQ2DaeQzGyF6wb+A6b8EQo52KhIBKYxJI
PperPXztPFMfKriNIrXNHsTYPWmXHIRr7F/XC1mooEdXPkOJZZJU6da6p0j5UXKY
RmAPMPdqf7Yo38cUXfOrVyJHCn9+HsjQHUyOts0Q/l2AqtW0jy1JIhTvfVVSbxbQ
SacNOg0rHYvRwveX0jWmMpUjaNNdhmSaFJUyyQIDAQABAoIBAD58zVX4MVVDkZu+
fbmelyimBtXDbC1nrbQspSifvfRD6KjSuyU8L/7cVm7Z+0drCgXrh5xRcjxP4Jn9
M2iui5QXyez2/96/B/kliLvAeeZRVI4/Bak22h1arDhWuw5nisyUNZDFg5W9c31/
9nFiJyvLyv4RtHOhUMnofVqUkCZ5AyYHvYQsILfLlLN+ZRWp46+ALEa2f0l0MyDQ
4cDAZKsmcrjzEim2N3277BsXNhMwzRgySjw/h3FJ21GtFe+4zTMQ/8SazHwoXetk
Wv8MaOGeJep1Y/oEujHOsneCRZ85RE/ijUNNjSbH5FDWsbJ8vJVL8qiomj/Q80RS
KZn8AAECgYEAzQQBTSxcFkMAmIz6kb795XoY4qtb5jbxLWMxiuM3bUu3wam7OIXo
ESe4EN7fY3vIdt4UJcKw4njKCX2QyN2iqGSqSmcRqe4dTQv0AzAGb5XausdEe4yw
9uWV2HGc0Rpyvhu/TRcUYzXp24rjdwXMVh51+yomrnQ+otJE1yMwHAECgYEAwrPg
rZn5uKdl1FfTNJCfWYL9fjqdhY8hgF3RLXARM3EUKPAYwtDqn+TE4c0FDZ0gSFkE
v+HdJTnDXNdVGdwaALeRttB1gYRbswZ9F3Cr9fnPuCYk9z5XYpldEkavgrqK5fTs
FlynPukvwc2AU8mVKLrC8Y7BPZZaX0M83gnnNskCgYBuH5+fV5ujbZwtVVTm4uO4
1wv0/bzgfVSxX53mD8TfFZQAF+70HqGYTXCGx2DRLFVy3DmQSvL+w4kq7eLOspbD
w0bhrlmDoN7mWuxYpfxfBey29YCoqNsJ1CrYV7a3b3CBA6CPhT0zSWtzvTgP3/Jx
s+0F3A9pGBHpHe9SbJlUAQKBgQC534k9kgIxgzWWWuWZh/toM8IWkJSy3WqJJoc8
ToFNT8WEM3of+dwTGw3N1rDdR0R7bOg42sII+LUF29g1YMc+KgEkuquDIr18ElS3
XOv9XigsT9X4Zv57dZfBi9OgBL/3OjNsQbW0PF9IMAwzcP1BrdHPU44tYm0SBpmE
C4Y86QKBgAmiOMI1dj74D2jQcbH4iIynAGsU61cqh1EJcRcGd7QvRXT9RKtnkanx
4A6S+UCSduS174s5Zp4wz5A/Neu9PlJj3dQi5SyjIOVjbU76pR3eyprxMY4Y1FnN
ZGUZbOpIIgjE70sz43TQBD+oGvOzcpS9tcbj3k8WlQUsLOczPIlV
-----END RSA PRIVATE KEY-----`
)

func clientCredentials(t testing.TB) credentials.TransportCredentials {
	t.Helper()
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM([]byte(serverCRT)) {
		t.Errorf("credentials: failed to append certificates")
	}
	return credentials.NewTLS(&tls.Config{ServerName: "", RootCAs: cp})
}

func serverCredentials(t testing.TB) credentials.TransportCredentials {
	t.Helper()
	cert, err := tls.X509KeyPair([]byte(serverCRT), []byte(serverKey))
	if err != nil {
		t.Error(err)
	}
	return credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}})
}

func TestSecureStorage(t *testing.T) {
	defer leakcheck.Check(t)
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
		},
		false,
		true,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(servers.addrs(), dialOpts, qc.WithDialTimeout(time.Second))
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	// Get all all available node ids
	ids := mgr.NodeIDs()
	// Quorum spec: rq=2. wq=3, n=3, sort by timestamp.
	qspec := NewStorageByTimestampQSpec(2, len(ids))
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	// Test state
	state := &qc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	// Perform write call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wreply, err := config.Write(ctx, state)
	if err != nil {
		t.Fatalf("write quorum call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.New {
		t.Error("write reply was not marked as new")
	}

	// Do read call
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rreply, err := config.Read(ctx, &qc.ReadRequest{})
	if err != nil {
		t.Fatalf("read quorum call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Value != state.Value {
		t.Errorf("read reply: want state %v, got %v", state, rreply)
	}

	nodes := mgr.Nodes()
	for _, m := range nodes {
		t.Logf("%v", m)
	}
}
