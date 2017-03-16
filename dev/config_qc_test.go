package dev_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"

	"golang.org/x/net/context"

	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
)

func TestMain(m *testing.M) {
	flag.Parse()
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false
	res := m.Run()
	os.Exit(res)
}

func TestBasicRegister(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgrOpts := []qc.ManagerOption{
		dialOpts,
		qc.WithTracing(),
	}
	mgr, err := qc.NewManager(
		servers.addrs(),
		mgrOpts...,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Quorum spec: rq=2. wq=3, n=3, sort by timestamp.
	qspec := NewRegisterByTimestampQSpec(2, len(ids))

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
		t.Fatalf("read reply: got state %v, want state %v", rreply.State, state)
	}

	nodes := mgr.Nodes()
	for _, m := range nodes {
		t.Logf("%v", m)
	}
}

func TestSingleServerRPC(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	state := &qc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	nodes := mgr.Nodes()
	ctx := context.Background()
	for _, node := range nodes {
		wreply, err := node.RegisterClient.Write(ctx, state)
		if err != nil {
			t.Fatalf("write quorum call error: %v", err)
		}
		if !wreply.New {
			t.Fatalf("write reply was not marked as new")
		}

		rreply, err := node.RegisterClient.ReadNoQC(ctx, &qc.ReadRequest{})
		if err != nil {
			t.Fatalf("read quorum call error: %v", err)
		}
		if rreply.Value != state.Value {
			t.Fatalf("read reply: got state %v, want %v", rreply.Value, state)
		}
	}
}

func TestExitHandleRepliesLoop(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	config, err := mgr.NewConfiguration(ids, &NeverQSpec{})
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	replyChan := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err = config.Read(ctx, &qc.ReadRequest{})
		replyChan <- err
	}()
	select {
	case err := <-replyChan:
		if err == nil {
			t.Fatalf("got no error, want one")
		}
		_, ok := err.(qc.QuorumCallError)
		if !ok {
			t.Fatalf("got error of type %T, want error of type %T\nerror details: %v", err, qc.QuorumCallError{}, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("read quorum call: timeout, call did not return")
	}
}

func TestSlowRegister(t *testing.T) {
	defer leakCheck(t)()
	someErr := grpc.Errorf(codes.Unknown, "Some error")
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterSlow(time.Second)},
			{impl: qc.NewRegisterSlow(time.Second)},
			{impl: qc.NewRegisterError(someErr)},
			// Q=2 below, one error server, two slow servers
			// -> must timeout with one error received.
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewMajorityQSpec(len(ids))
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = config.Read(ctx, &qc.ReadRequest{})
	if err == nil {
		t.Fatalf("read quorum call: got no error, want one")
	}
	timeoutErr, ok := err.(qc.QuorumCallError)
	if !ok {
		t.Fatalf("got error of type %T, want error of type %T\nerror details: %v", err, qc.QuorumCallError{}, err)
	}
	wantErrReason := "context deadline exceeded"
	if timeoutErr.Reason != wantErrReason {
		t.Fatalf("got error reason: %v, want: %v", timeoutErr.Reason, wantErrReason)
	}
}

func TestBasicRegisterUsingFuture(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewRegisterQSpec(1, len(ids))
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	// Write asynchronously.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	wfuture := config.WriteFuture(ctx, state)

	// Wait for all writes to finish.
	servers.waitForAllWrites()

	wreply, err := wfuture.Get()
	if err != nil {
		t.Fatalf("write future quorum call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.New {
		t.Fatalf("write future reply was not marked as new")
	}

	// Done should report true.
	done := wfuture.Done()
	if !done {
		t.Fatalf("write future was not done")
	}

	// Read asynchronously.
	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	rfuture := config.ReadFuture(ctx, &qc.ReadRequest{})

	// Inspect read reply when available.
	rreply, err := rfuture.Get()
	if err != nil {
		t.Fatalf("read future quorum call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Value != state.Value {
		t.Fatalf("read future reply:\ngot:\n%v\nwant:\n%v", rreply, state)
	}
}

func TestBasicRegisterWithWriteAsync(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewRegisterQSpec(1, len(ids))

	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	stateOne := &qc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	wreply, err := config.Write(ctx, stateOne)
	if err != nil {
		t.Fatalf("write quorum call error: %v", err)
	}

	// Drain all writers after synchronous call.
	servers.waitForAllWrites()

	t.Logf("wreply: %v\n", wreply)
	if !wreply.New {
		t.Fatal("write reply was not marked as new")
	}

	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	rreply, err := config.Read(ctx, &qc.ReadRequest{})
	if err != nil {
		t.Fatalf("read quorum call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Value != stateOne.Value {
		t.Fatalf("read reply:\ngot:\n%v\nwant:\n%v", rreply.State, stateOne)
	}

	stateTwo := &qc.State{
		Value:     "99",
		Timestamp: time.Now().UnixNano(),
	}

	// Write a value using the WriteAsync stream.
	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	err = config.WriteAsync(ctx, stateTwo)
	if err != nil {
		t.Fatalf("write-async quorum call error: %v", err)
	}

	// Wait for all writes to finish.
	servers.waitForAllWrites()

	ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	rreply, err = config.Read(ctx, &qc.ReadRequest{})
	if err != nil {
		t.Fatalf("read quorum call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Value != stateTwo.Value {
		t.Fatalf("read reply:\ngot:\n%v\nwant:\n%v", rreply.State, stateTwo)
	}
}

func TestManagerClose(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}

	const timeoutDur = time.Second
	closeReturnedChan := make(chan struct{}, 1)

	go func() {
		mgr.Close()
		close(closeReturnedChan)
	}()

	select {
	case <-closeReturnedChan:
	case <-time.After(timeoutDur):
		t.Fatalf("mgr.Close() timed out, waited %v", timeoutDur)
	}
}

func TestQuorumCallCancel(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterSlow(time.Second)},
			{impl: qc.NewRegisterSlow(time.Second)},
			{impl: qc.NewRegisterSlow(time.Second)},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewMajorityQSpec(len(ids))
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel() // Main point: cancel at once, not defer.
	_, err = config.Read(ctx, &qc.ReadRequest{})
	if err == nil {
		t.Fatalf("read quorum call: got no error, want one")
	}
	err, ok := err.(qc.QuorumCallError)
	if !ok {
		t.Fatalf("got error of type %T, want error of type %T\nerror details: %v", err, qc.QuorumCallError{}, err)
	}
	wantErr := qc.QuorumCallError{Reason: "context canceled", ErrCount: 0, ReplyCount: 0}
	if err != wantErr {
		t.Fatalf("got error: %v, want: %v", err, wantErr)
	}
}

func TestBasicCorrectable(t *testing.T) {
	defer leakCheck(t)()

	stateOne := &qc.State{
		Value:     "42",
		Timestamp: 1,
	}
	stateTwo := &qc.State{
		Value:     "99",
		Timestamp: 100,
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterSlowWithState(5*time.Millisecond, stateOne)},
			{impl: qc.NewRegisterSlowWithState(20*time.Millisecond, stateTwo)},
			{impl: qc.NewRegisterSlowWithState(500*time.Millisecond, stateTwo)},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	majority := len(ids)/2 + 1
	qspec := NewRegisterByTimestampQSpec(majority, majority)
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	correctable := config.ReadCorrectable(ctx, &qc.ReadRequest{})

	select {
	case <-correctable.Done():
		reply, level, err := correctable.Get()
		if err != nil {
			t.Fatalf("read correctable call: get: got error: %v, want none", err)
		}
		if level != LevelStrong {
			t.Fatalf("read correctable: get: got level %v, want %v", level, LevelStrong)
		}
		if reply.Value != stateTwo.Value {
			t.Fatalf("read correctable: get: reply:\ngot:\n%v\nwant:\n%v", reply.Value, stateTwo)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("read correctable: was not done and call did not timeout using context")
	}
}

func TestCorrectableWithLevels(t *testing.T) {
	defer leakCheck(t)()

	stateOne := &qc.State{
		Value:     "42",
		Timestamp: 1,
	}
	stateTwo := &qc.State{
		Value:     "99",
		Timestamp: 100,
	}

	// We need the specific implementation so we call the Unlock method.
	regServersImplementation := []*qc.RegisterServerLockedWithState{
		qc.NewRegisterServerLockedWithState(stateOne, 0),
		qc.NewRegisterServerLockedWithState(stateTwo, 0),
		qc.NewRegisterServerLockedWithState(stateTwo, 0),
	}

	regServersInterface := []regServer{
		{impl: regServersImplementation[0]},
		{impl: regServersImplementation[1]},
		{impl: regServersImplementation[2]},
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		regServersInterface,
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	majority := len(ids)/2 + 1
	qspec := NewRegisterByTimestampQSpec(majority, majority)
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	waitTimeout := time.Second
	ctx := context.Background()
	correctable := config.ReadCorrectable(ctx, &qc.ReadRequest{})

	// Watch for level 1 ("weak") for this quorum specification.
	levelOneChan := correctable.Watch(LevelWeak)

	// Watch for level 5 (undefined). Higher than any level used in the
	// quorum function. The channel should still be closed when the call
	// completes.
	levelFiveChan := correctable.Watch(5)

	// Initial check: no server has replied yet.
	// Check that the level 1 watch channel is not done.
	select {
	case <-levelOneChan:
		t.Fatalf("read correctable: level 1 (weak) chan was closed before any reply was received")
	default:
	}

	// Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctable: Done() was done before any reply was received")
	default:
	}

	// Check that Get() returns nil, LevelNotSet, nil.
	reply, level, err := correctable.Get()
	if err != nil {
		t.Fatalf("read correctable: initial get: got unexpected error: %v", err)
	}
	if level != qc.LevelNotSet {
		t.Fatalf("read correctable: initial get: got level %v, want %v", level, qc.LevelNotSet)
	}
	if reply != nil {
		t.Fatal("read correctable: initial get: got reply, want none")
	}

	// Unlock server with lowest timestamp for state.
	regServersImplementation[0].Unlock()

	// Wait for level 1 (weak) notification.
	select {
	case <-levelOneChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable: waiting for levelOneChan timed out (waited %v)", waitTimeout)
	}

	// Check that Get() returns stateOne, LevelWeak, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable: get after one reply: got unexpected error: %v", err)
	}
	if level != LevelWeak {
		t.Fatalf("read correctable: get after one reply: got level %v, want %v", level, LevelWeak)
	}
	if reply.Value != stateOne.Value {
		t.Fatalf("read correctable: get after one reply:\ngot reply:\n%v\nwant:\n%v", reply.Value, stateOne.Value)
	}

	// Unlock both of the two servers with the highest timestamp for state.
	regServersImplementation[1].Unlock()
	regServersImplementation[2].Unlock()

	// Wait for Done channel notification.
	select {
	case <-correctable.Done():
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable: waiting for Done channel timed out (waited %v)", waitTimeout)
	}

	// Check that Get() returns stateTwo, LevelStrong, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable: get after done call: got unexpected error: %v", err)
	}
	if level != LevelStrong {
		t.Fatalf("read correctable: get after done call: got level %v, want %v", level, LevelStrong)
	}
	if reply.Value != stateTwo.Value {
		t.Fatalf("read correctable: get after done call:\ngot reply:\n%v\nwant:\n%v", reply.Value, stateTwo.Value)
	}

	// Check that channel for level 5 (undefined) notification is closed.
	select {
	case <-levelFiveChan:
	default:
		t.Fatal("read correctable: call is complete but levelFive notification channel was not closed", waitTimeout)
	}
}

func TestCorrectablePrelim(t *testing.T) {
	defer leakCheck(t)()

	stateOne := &qc.State{
		Value:     "42",
		Timestamp: 1,
	}
	stateTwo := &qc.State{
		Value:     "99",
		Timestamp: 100,
	}

	// We need the specific implementation so we call the Unlock and PerformReadPrelimChan methods.
	regServersImplementation := []*qc.RegisterServerLockedWithState{
		qc.NewRegisterServerLockedWithState(stateOne, 2),
		qc.NewRegisterServerLockedWithState(stateTwo, 2),
		qc.NewRegisterServerLockedWithState(stateTwo, 0),
	}

	regServersInterface := []regServer{
		{impl: regServersImplementation[0]},
		{impl: regServersImplementation[1]},
		{impl: regServersImplementation[2]},
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		regServersInterface,
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	config, err := mgr.NewConfiguration(ids, &ReadPrelimTestQSpec{})
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	waitTimeout := time.Second
	ctx := context.Background()
	correctable := config.ReadPrelim(ctx, &qc.ReadRequest{})

	// We need these watchers for testing to know that a server has replied and
	// gorums has processed the reply.
	levelOneChan := correctable.Watch(1)
	levelTwoChan := correctable.Watch(2)
	levelThreeChan := correctable.Watch(3)
	levelFourChan := correctable.Watch(4)

	// Unlock all servers.
	regServersImplementation[0].Unlock()
	regServersImplementation[1].Unlock()
	regServersImplementation[2].Unlock()

	// It should be possible to reduce code duplication below.
	// TODO: DRY.

	// 0.1: Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctablae prelim: Done() was done before any reply was received")
	default:
	}

	// 0.2: Check that Get() returns nil, LevelNotSet, nil.
	reply, level, err := correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: initial get: got unexpected error: %v", err)
	}
	if level != qc.LevelNotSet {
		t.Fatalf("read correctable prelim: initial get: got level %v, want %v", level, qc.LevelNotSet)
	}
	if reply != nil {
		t.Fatal("read correctable prelim: initial get: got reply, want none")
	}

	regServersImplementation[0].PerformSingleReadPrelim()

	// Wait for level 1 notification.
	select {
	case <-levelOneChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for levelOneChan timed out (waited %v)", waitTimeout)
	}

	// 1.1: Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctablae prelim: Done() was done at level 1")
	default:
	}

	// 1.2: Check that Get() returns stateTwo, 3, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: get (1): got unexpected error: %v", err)
	}
	if level != 1 {
		t.Fatalf("read correctable prelim: get (1): got level %v, want %v", level, 1)
	}
	if reply.Value != stateOne.Value {
		t.Fatalf("read correctable prelim: get (1):\ngot reply:\n%v\nwant:\n%v", reply.Value, stateOne.Value)
	}

	regServersImplementation[0].PerformSingleReadPrelim()

	// Wait for level 2 notification.
	select {
	case <-levelTwoChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for levelTwoChan timed out (waited %v)", waitTimeout)
	}

	// 2.2: Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctablae prelim: Done() was done at level 2")
	default:
	}

	// 2.2: Check that Get() returns stateTwo, 3, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: get (2): got unexpected error: %v", err)
	}
	if level != 2 {
		t.Fatalf("read correctable prelim: get (2): got level %v, want %v", level, 2)
	}
	if reply.Value != stateOne.Value {
		t.Fatalf("read correctable prelim: get (2):\ngot reply:\n%v\nwant:\n%v", reply.Value, stateOne.Value)
	}

	regServersImplementation[1].PerformSingleReadPrelim()

	// Wait for level 3 notification.
	select {
	case <-levelThreeChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for levelThreeChan timed out (waited %v)", waitTimeout)
	}

	// 3.1: Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctablae prelim: Done() was done at level 3")
	default:
	}

	// 3.2: Check that Get() returns stateTwo, 3, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: get (3): got unexpected error: %v", err)
	}
	if level != 3 {
		t.Fatalf("read correctable prelim: get (3): got level %v, want %v", level, 3)
	}
	if reply.Value != stateTwo.Value {
		t.Fatalf("read correctable prelim: get (3):\ngot reply:\n%v\nwant:\n%v", reply.Value, stateTwo.Value)
	}

	regServersImplementation[1].PerformSingleReadPrelim()

	// Wait for level 4 notification.
	select {
	case <-levelFourChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for levelFourChan timed out (waited %v)", waitTimeout)
	}

	// 4.1: Check that Done() is done.
	select {
	case <-correctable.Done():
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for Done channel timed out (waited %v)", waitTimeout)
	}

	// 4.2: Check that Get() returns stateTwo, 4, nil.
	reply, level, err = correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: final get: got unexpected error: %v", err)
	}
	if level != 4 {
		t.Fatalf("read correctable prelim: final get: got level %v, want %v", level, 4)
	}
	if reply.Value != stateTwo.Value {
		t.Fatalf("read correctable prelim: get after done call:\ngot reply:\n%v\nwant:\n%v", reply.Value, stateTwo.Value)
	}
}

func TestPerNodeArg(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
			{impl: qc.NewRegisterBasic()},
		},
		false,
	)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgrOpts := []qc.ManagerOption{
		dialOpts,
		qc.WithTracing(),
	}
	mgr, err := qc.NewManager(
		servers.addrs(),
		mgrOpts...,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Quorum spec: rq=2. wq=3, n=3, sort by timestamp.
	qspec := NewRegisterByTimestampQSpec(2, len(ids))

	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	// Construct test state
	state := make(map[uint32]*qc.State, len(ids))
	for _, n := range ids {
		state[n] = &qc.State{Value: fmt.Sprintf("%d", n), Timestamp: time.Now().UnixNano()}
	}

	// Perform write per node arg call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req := &qc.State{}

	var cnt int64
	perNodeArg := func(req qc.State, nodeID uint32) *qc.State {
		if atomic.LoadInt64(&cnt) > 1 {
			time.Sleep(250 * time.Millisecond)
			t.Logf("delay sending to node %d\n", nodeID)
		}
		atomic.AddInt64(&cnt, 1)
		t.Logf("sending to node %d\n", nodeID)
		req.Value = fmt.Sprintf("%d", nodeID)
		req.Timestamp = time.Now().UnixNano()
		return &req
	}
	wreply, err := config.WritePerNode(ctx, req, perNodeArg)
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
	valNodeID, _ := strconv.ParseUint(rreply.Value, 10, 32)
	if rreply.State.Value != state[uint32(valNodeID)].Value {
		t.Fatalf("read reply: got state %v, want state %v", rreply.State.Value, state[uint32(valNodeID)].Value)
	}

	// Test nil return value from perNodeArg to indicate that we should ignore a node

	/* DISABLED TODO need to implement logic to test for nil in quorumcall.tmpl
	nodeToIgnore := mgr.NodeIDs()[0]
	perNodeArgNil := func(req qc.State, nodeID uint32) *qc.State {
		if nodeID == nodeToIgnore {
			t.Logf("ignoring node %d\n", nodeID)
			return nil
		}
		t.Logf("sending to node %d\n", nodeID)
		req.Value = fmt.Sprintf("%d", nodeID)
		req.Timestamp = time.Now().UnixNano()
		return &req
	}
	wreply, err = config.WritePerNode(ctx, req, perNodeArgNil)
	if err != nil {
		t.Fatalf("write quorum call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.New {
		t.Error("write reply was not marked as new")
	}
	*/

}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KQ1N3Local(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, false, false, false)
}

func BenchmarkRead1KQ1N3Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, false, false, true)
}

func BenchmarkRead1KQ1N3ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, true, false, false)
}

func BenchmarkRead1KQ1N3ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, true, false, true)
}

func BenchmarkRead1KQ1N3FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, false, true, false)
}

func BenchmarkRead1KQ1N3FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, false, true, true)
}

func BenchmarkRead1KQ1N3FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, true, true, false)
}

func BenchmarkRead1KQ1N3FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, false, true, true, true)
}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KQ2N3Local(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, false, false, false)
}

func BenchmarkRead1KQ2N3Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, false, false, true)
}

func BenchmarkRead1KQ2N3ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, true, false, false)
}

func BenchmarkRead1KQ2N3ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, true, false, true)
}

func BenchmarkRead1KQ2N3FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, false, true, false)
}

func BenchmarkRead1KQ2N3FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, false, true, true)
}

func BenchmarkRead1KQ2N3FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, true, true, false)
}

func BenchmarkRead1KQ2N3FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, false, true, true, true)
}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KQ1N1Local(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, false, false, false)
}

func BenchmarkRead1KQ1N1Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, false, false, true)
}

func BenchmarkRead1KQ1N1ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, true, false, false)
}

func BenchmarkRead1KQ1N1ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, true, false, true)
}

func BenchmarkRead1KQ1N1FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, false, true, false)
}

func BenchmarkRead1KQ1N1FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, false, true, true)
}

func BenchmarkRead1KQ1N1FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, true, true, false)
}

func BenchmarkRead1KQ1N1FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, true, true, true, true)
}

///////////////////////////////////////////////////////////////

var replySink *qc.ReadReply
var replySinkFuture *qc.State

func benchmarkRead(b *testing.B, size, rq int, single, parallel, future, remote bool) {
	var rservers []regServer
	if !remote {
		rservers = []regServer{
			{impl: qc.NewRegisterBench()},
		}
		if !single {
			rservers = append(
				rservers,
				regServer{impl: qc.NewRegisterBench()},
				regServer{impl: qc.NewRegisterBench()},
			)
		}
	} else {
		rservers = []regServer{
			{addr: "pitter31:8080"},
		}
		if !single {
			rservers = append(
				rservers,
				regServer{},
				regServer{},
			)
		}
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(b, rservers, remote)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		b.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewRegisterQSpec(rq, len(ids))
	ctx := context.Background()
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		b.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     strings.Repeat("x", size),
		Timestamp: time.Now().UnixNano(),
	}

	wreply, err := config.Write(ctx, state)
	if err != nil {
		b.Fatalf("write quorum call error: %v", err)
	}
	if !wreply.New {
		b.Fatalf("intital write reply was not marked as new")
	}

	b.SetBytes(int64(state.Size()))
	b.ReportAllocs()
	b.ResetTimer()

	if !future {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					replySink, err = config.Read(ctx, &qc.ReadRequest{})
					if err != nil {
						b.Fatalf("read quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				replySink, err = config.Read(ctx, &qc.ReadRequest{})
				if err != nil {
					b.Fatalf("read quorum call error: %v", err)
				}
			}
		}
	} else {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					rf := config.ReadFuture(ctx, &qc.ReadRequest{})
					replySinkFuture, err = rf.Get()
					if err != nil {
						b.Fatalf("read future quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				rf := config.ReadFuture(ctx, &qc.ReadRequest{})
				replySinkFuture, err = rf.Get()
				if err != nil {
					b.Fatalf("read future quorum call error: %v", err)
				}
			}
		}
	}
}

///////////////////////////////////////////////////////////////

func BenchmarkWrite1KQ2N3Local(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, false, false, false)
}

func BenchmarkWrite1KQ2N3Remote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, false, false, true)
}

func BenchmarkWrite1KQ2N3ParallelLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, true, false, false)
}

func BenchmarkWrite1KQ2N3ParallelRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, true, false, true)
}

func BenchmarkWrite1KQ2N3FutureLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, false, true, false)
}

func BenchmarkWrite1KQ2N3FutureRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, false, true, true)
}

func BenchmarkWrite1KQ2N3FutureParallelLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, true, true, false)
}

func BenchmarkWrited1KQ2N3FutureParallelRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, false, true, true, true)
}

///////////////////////////////////////////////////////////////

var wreplySink *qc.WriteReply
var wreplySinkFuture *qc.WriteResponse

func benchmarkWrite(b *testing.B, size, wq int, single, parallel, future, remote bool) {
	var rservers []regServer
	if !remote {
		rservers = []regServer{
			{impl: qc.NewRegisterBench()},
		}
		if !single {
			rservers = append(
				rservers,
				regServer{impl: qc.NewRegisterBench()},
				regServer{impl: qc.NewRegisterBench()},
			)
		}
	} else {
		rservers = []regServer{
			{addr: "pitter31:8080"},
		}
		if !single {
			rservers = append(
				rservers,
				regServer{},
				regServer{},
			)
		}
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(b, rservers, remote)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	mgr, err := qc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		b.Fatalf("%v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec := NewRegisterQSpec(0, wq)
	ctx := context.Background()
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		b.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     strings.Repeat("x", size),
		Timestamp: time.Now().UnixNano(),
	}

	b.SetBytes(int64(state.Size()))
	b.ReportAllocs()
	b.ResetTimer()

	if !future {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					wreplySink, err = config.Write(ctx, state)
					if err != nil {
						b.Fatalf("write quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				wreplySink, err = config.Write(ctx, state)
				if err != nil {
					b.Fatalf("write quorum call error: %v", err)
				}
			}
		}
	} else {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					rf := config.WriteFuture(ctx, state)
					wreplySinkFuture, err = rf.Get()
					if err != nil {
						b.Fatalf("write future quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				rf := config.WriteFuture(ctx, state)
				wreplySinkFuture, err = rf.Get()
				if err != nil {
					b.Fatalf("write future quorum call error: %v", err)
				}
			}
		}
	}
}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KGRPCLocal(b *testing.B)          { benchReadGRPC(b, 1<<10, false, false) }
func BenchmarkRead1KGRPCRemote(b *testing.B)         { benchReadGRPC(b, 1<<10, false, true) }
func BenchmarkRead1KGRPCParallelLocal(b *testing.B)  { benchReadGRPC(b, 1<<10, true, false) }
func BenchmarkRead1KGRPCParallelRemote(b *testing.B) { benchReadGRPC(b, 1<<10, true, true) }

var grpcReplySink *qc.State

func benchReadGRPC(b *testing.B, size int, parallel, remote bool) {
	var rservers []regServer
	if !remote {
		rservers = []regServer{
			{impl: qc.NewRegisterBench()},
		}
	} else {
		rservers = []regServer{
			{addr: "pitter33:8080"},
		}
	}

	servers, _, stopGrpcServe, closeListeners := setup(b, rservers, remote)
	defer closeListeners(allServers)
	defer stopGrpcServe(allServers)

	conn, err := grpc.Dial(servers.addrs()[0], grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second))
	if err != nil {
		b.Fatalf("grpc dial: %v", err)
	}

	state := &qc.State{
		Value:     strings.Repeat("x", size),
		Timestamp: time.Now().UnixNano(),
	}

	rclient := qc.NewRegisterClient(conn)
	ctx := context.Background()

	reply, err := rclient.Write(ctx, state)
	if err != nil {
		b.Fatalf("write quorum call error: %v", err)
	}
	if !reply.New {
		b.Fatalf("intital write reply was not marked as new")
	}

	b.SetBytes(int64(state.Size()))
	b.ReportAllocs()
	b.ResetTimer()

	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				grpcReplySink, err = rclient.Read(ctx, &qc.ReadRequest{})
				if err != nil {
					b.Fatalf("read quorum call error: %v", err)
				}
			}
		})
	} else {
		for i := 0; i < b.N; i++ {
			grpcReplySink, err = rclient.Read(ctx, &qc.ReadRequest{})
			if err != nil {
				b.Fatalf("read quorum call error: %v", err)
			}
		}
	}
}

const allServers = -1

var portSupplier = struct {
	p int
	sync.Mutex
}{p: 22332}

func setup(t testing.TB, regServers []regServer, remote bool) (regServers, qc.ManagerOption, func(n int), func(n int)) {
	if len(regServers) == 0 {
		t.Fatal("setupServers: need at least one server")
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(time.Second),
		grpc.WithInsecure(),
	}
	dialOpts := qc.WithGrpcDialOptions(grpcOpts...)

	if remote {
		return regServers, dialOpts, func(int) {}, func(int) {}
	}

	servers := make([]*grpc.Server, len(regServers))
	for i := range servers {
		servers[i] = grpc.NewServer()
		qc.RegisterRegisterServer(servers[i], regServers[i].impl)
		if regServers[i].addr == "" {
			portSupplier.Lock()
			regServers[i].addr = fmt.Sprintf(":%d", portSupplier.p)
			portSupplier.p++
			portSupplier.Unlock()
		}
	}

	listeners := make([]net.Listener, len(servers))

	var err error
	for i, rs := range regServers {
		listeners[i], err = net.Listen("tcp", rs.addr)
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
	}
	for i, server := range servers {
		go func(i int, server *grpc.Server) {
			_ = server.Serve(listeners[i])
		}(i, server)
	}

	for i, listener := range listeners {
		_, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			t.Fatalf("failed to parse listener address: %v", err)
		}
		regServers[i].addr = "localhost:" + port
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

	return regServers, dialOpts, stopGrpcServeFunc, closeListenersFunc
}

type regServer struct {
	impl qc.RegisterTestServer
	addr string
}

type regServers []regServer

func (rs regServers) addrs() []string {
	addrs := make([]string, len(rs))
	for i, server := range rs {
		addrs[i] = server.addr
	}
	return addrs
}

// waitForAllWrites must only be called when
// all servers in rs performs the write operation.
// Calling this method when only using a subset of servers
// is an error.
//
// TODO: Adjust this to allow waiting for a
// subset of writes.
func (rs regServers) waitForAllWrites() {
	for _, server := range rs {
		server.impl.WriteExecuted()
	}
}

type ByTimestamp []*qc.State

func (a ByTimestamp) Len() int           { return len(a) }
func (a ByTimestamp) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTimestamp) Less(i, j int) bool { return a[i].Timestamp < a[j].Timestamp }

// leakCheck snapshots the currently-running goroutines and returns a
// function to be run at the end of tests to see whether any
// goroutines leaked.
//
// From https://github.com/grpc/grpc-go
// Copyright 2014, Google Inc.
func leakCheck(t testing.TB) func() {
	orig := map[string]bool{}
	for _, g := range interestingGoroutines() {
		orig[g] = true
	}
	return func() {
		// Loop, waiting for goroutines to shut down.
		// Wait up to 5 seconds, but finish as quickly as possible.
		deadline := time.Now().Add(5 * time.Second)
		for {
			var leaked []string
			for _, g := range interestingGoroutines() {
				if !orig[g] {
					leaked = append(leaked, g)
				}
			}
			if len(leaked) == 0 {
				return
			}
			if time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for _, g := range leaked {
				t.Errorf("Leaked goroutine: %v", g)
			}
			return
		}
	}
}

// interestingGoroutines returns all goroutines we care about for the purpose
// of leak checking. It excludes testing or runtime ones.
//
// From https://github.com/grpc/grpc-go
// Copyright 2014, Google Inc.
func interestingGoroutines() (gs []string) {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	for _, g := range strings.Split(string(buf), "\n\n") {
		sl := strings.SplitN(g, "\n", 2)
		if len(sl) != 2 {
			continue
		}
		stack := strings.TrimSpace(sl[1])
		if stack == "" ||
			strings.Contains(stack, "testing.runTests") ||
			strings.Contains(stack, "testing.RunTests") ||
			strings.Contains(stack, "testing.Main(") ||
			strings.Contains(stack, "runtime.goexit") ||
			strings.Contains(stack, "created by runtime.gc") ||
			strings.Contains(stack, "interestingGoroutines") ||
			strings.Contains(stack, "runtime.MHeap_Scavenger") ||
			strings.Contains(stack, "signal.signal_recv") ||
			strings.Contains(stack, "sigterm.handler") ||
			strings.Contains(stack, "runtime_mcall") ||
			strings.Contains(stack, "goroutine in C code") {
			continue
		}
		gs = append(gs, g)
	}
	sort.Strings(gs)
	return
}
