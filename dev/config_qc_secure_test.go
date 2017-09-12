package dev_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// TODO(meling):
//
// Implement functionality to keep certs and keys in appropriate places.
//
// Figure out how to generate certs and keys (currently these are only
// google's certs and keys).
//
// Clean up this stuff; merge with other test setup function or something
// smart; perhaps splitting it up into separate functions will help with reuse.

const (
	tlsDir         = "../testdata/tls/"
	serverCertFile = tlsDir + "server1.pem"
	serverKeyFile  = tlsDir + "server1.key"
)

func secsetup(t testing.TB, srvs regServers) (func(n int), func(n int)) {
	if len(srvs) == 0 {
		t.Fatal("setupServers: need at least one server")
	}

	var err error
	servers := make([]*grpc.Server, len(srvs))
	listeners := make([]net.Listener, len(srvs))
	for i, rs := range srvs {
		if rs.addr == "" {
			portSupplier.Lock()
			rs.addr = fmt.Sprintf("localhost:%d", portSupplier.p)
			portSupplier.p++
			portSupplier.Unlock()
		}
		listeners[i], err = net.Listen("tcp", rs.addr)
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}

		//todo(meling) should store credentials in Manager or Node
		creds, err := credentials.NewServerTLSFromFile(serverCertFile, serverKeyFile)
		if err != nil {
			t.Fatalf("failed to generate credentials %v", err)
		}
		opts := []grpc.ServerOption{grpc.Creds(creds)}
		servers[i] = grpc.NewServer(opts...)
		qc.RegisterStorageServer(servers[i], srvs[i].impl)

		go func(i int, server *grpc.Server) {
			_ = server.Serve(listeners[i])
		}(i, servers[i])
		srvs[i].addr = rs.addr
	}

	stopGrpcServeFunc := func(n int) {
		if n < 0 || n > len(servers) {
			for _, s := range servers {
				s.Stop()
			}
		} else {
			servers[n].Stop()
		}
	}

	closeListenersFunc := func(n int) {
		if n < 0 || n > len(listeners) {
			for _, l := range listeners {
				l.Close()
			}
		} else {
			listeners[n].Close()
		}
	}

	return stopGrpcServeFunc, closeListenersFunc
}

func TestSecureStorage(t *testing.T) {
	defer leakCheck(t)()
	servers := regServers{
		{impl: qc.NewStorageBasic()},
		{impl: qc.NewStorageBasic()},
		{impl: qc.NewStorageBasic()},
	}
	stopGrpcServe, closeListeners := secsetup(t, servers)
	defer stopGrpcServe(allServers)

	//TODO fix hardcoded youtube server name (can we get certificate for localhost servername?)
	clientCreds, err := credentials.NewClientTLSFromFile(tlsDir+"ca.pem", "x.test.youtube.com")
	if err != nil {
		t.Errorf("error creating credentials: %v", err)
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(time.Second),
		grpc.WithTransportCredentials(clientCreds),
	}
	dialOpts := qc.WithGrpcDialOptions(grpcOpts...)

	mgr, err := qc.NewManager(servers.addrs(), dialOpts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

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
