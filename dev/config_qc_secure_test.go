package dev_test

import (
	"context"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"
)

// TODO(meling):
//
// Implement functionality to keep certs and keys in appropriate places.
//
// Figure out how to generate certs and keys (currently these are only
// google's certs and keys).

const (
	tlsDir         = "../testdata/tls/"
	serverCertFile = tlsDir + "server1.pem"
	serverKeyFile  = tlsDir + "server1.key"
)

func TestSecureStorage(t *testing.T) {
	defer leakCheck(t)()
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

	mgr, err := qc.NewManager(servers.addrs(), dialOpts)
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
