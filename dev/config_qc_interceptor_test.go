package dev_test

import (
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"
	"github.com/relab/gorums/internal/leakcheck"
	"golang.org/x/net/context"
)

func TestInterceptor(t *testing.T) {
	defer leakcheck.Check(t)
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
		},
		false,
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(servers.addrs(),
		dialOpts,
		qc.WithTracing(),
		qc.WithDialTimeout(time.Second),
	)
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
	if !wreply.New {
		t.Error("write reply was not marked as new")
	}

	// Do read call
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rreply, err := config.Read(ctx, &qc.ReadRequest{})
	if err != nil {
		t.Errorf("read quorum call error: %v", err)
	}
	if rreply.Value != state.Value {
		t.Errorf("read reply: got state %v, want state %v", rreply, state)
	}

	// Do read call with nil argument; this works and returns a reply
	// because the ReadRequest{} is never used on the server side.
	rreply, err = config.Read(ctx, nil)
	if rreply == nil {
		t.Error("expected non-nil reply")
	}
	if err != nil {
		t.Errorf("read quorum call error: %v", err)
	}
	if rreply != nil && rreply.Value != state.Value {
		t.Errorf("read reply: got state %v, want state %v", rreply, state)
	}

	// Perform write call with nil argument; expect a panic since the
	// the server side needs the State{} object.
	// TODO this test does not work because the Write() call creates
	// multiple goroutines, and it is one of these other goroutines that
	// causes the panic, and so this recover() call does not work.
	// defer func() {
	// 	if r := recover(); r == nil {
	// 		t.Fatalf("expected panic for nil argument to Write function")
	// 	}
	// }()
	// config.Write(ctx, nil)
}
