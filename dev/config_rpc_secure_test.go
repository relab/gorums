package dev_test

import (
	"net"
	"testing"
	"time"

	rpc "github.com/relab/gorums/dev"

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

func secsetup(t testing.TB, srvs regServers, remote bool) (func(n int), func(n int)) {
	if len(srvs) == 0 {
		t.Fatal("setupServers: need at least one server")
	}

	if remote {
		return func(int) {}, func(int) {}
	}

	var err error
	servers := make([]*grpc.Server, len(srvs))
	listeners := make([]net.Listener, len(srvs))
	for i, rs := range srvs {
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
		rpc.RegisterRegisterServer(servers[i], srvs[i].implementation)

		go func(i int, server *grpc.Server) {
			_ = server.Serve(listeners[i])
		}(i, servers[i])
		srvs[i].addr = "localhost" + rs.addr
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

func TestSecureRegister(t *testing.T) {
	defer leakCheck(t)()
	servers := regServers{
		{":8080", rpc.NewRegisterBasic()},
		{":8081", rpc.NewRegisterBasic()},
		{":8082", rpc.NewRegisterBasic()},
	}
	stopGrpcServe, closeListeners := secsetup(t, servers, false)
	defer stopGrpcServe(allServers)

	//TODO fix hardcoded youtube server name (can we get certificate for localhost servername?)
	clientCreds, err := credentials.NewClientTLSFromFile(tlsDir+"ca.pem", "x.test.youtube.com")
	if err != nil {
		t.Errorf("error creating credentials: %v", err)
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(50 * time.Millisecond),
		grpc.WithTransportCredentials(clientCreds),
	}
	dialOpts := rpc.WithGrpcDialOptions(grpcOpts...)

	mgr, err := rpc.NewManager(servers.addrs(), dialOpts)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Quorum spec: rq=2. wq=3, n=3, sort by timestamp.
	qspec := NewRegisterByTimestampQSpec(2, len(ids))

	config, err := mgr.NewConfiguration(ids, qspec, time.Second)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	// Test state
	state := &rpc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	// Perfomr write call
	wreply, err := config.Write(state)
	if err != nil {
		t.Fatalf("write rpc call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.Reply.New {
		t.Error("write reply was not marked as new")
	}

	// Do read call
	rreply, err := config.Read(&rpc.ReadRequest{})
	if err != nil {
		t.Fatalf("read rpc call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Reply.Value != state.Value {
		t.Errorf("read reply: want state %v, got %v", state, rreply.Reply)
	}

	nodes := mgr.Nodes(false)
	for _, m := range nodes {
		t.Logf("%v", m)
	}
}
