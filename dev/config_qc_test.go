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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qc "github.com/relab/gorums/dev"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
)

func TestMain(m *testing.M) {
	// Flag definitions.
	var hosts = flag.String(
		"remotehosts",
		"",
		"comma separated list of 'addr:port' pairs to use as hosts for remote benchmarks",
	)

	// Parse and validate flags.
	flag.Parse()
	err := parseHostnames(*hosts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	// Disable gRPC tracing and logging.
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false

	// Run tests/benchmarks.
	res := m.Run()
	os.Exit(res)
}

func TestBasicStorage(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
		t.Fatalf("read reply: got state %v, want state %v", rreply, state)
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
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
		wreply, err := node.StorageClient.Write(ctx, state)
		if err != nil {
			t.Fatalf("write quorum call error: %v", err)
		}
		if !wreply.New {
			t.Fatalf("write reply was not marked as new")
		}

		rreply, err := node.StorageClient.ReadNoQC(ctx, &qc.ReadRequest{})
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
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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

func TestSlowStorage(t *testing.T) {
	defer leakCheck(t)()
	someErr := grpc.Errorf(codes.Unknown, "Some error")
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageSlow(time.Second)},
			{impl: qc.NewStorageSlow(time.Second)},
			{impl: qc.NewStorageError(someErr)},
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

func TestBasicStorageUsingFuture(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
	qspec := NewStorageQSpec(1, len(ids))
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

func TestBasicStorageWithWriteAsync(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
	qspec := NewStorageQSpec(1, len(ids))

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
		t.Fatalf("read reply:\ngot:\n%v\nwant:\n%v", rreply, stateOne)
	}

	stateTwo := &qc.State{
		Value:     "99",
		Timestamp: time.Now().UnixNano(),
	}

	// Write a value using the WriteAsync stream.
	err = config.WriteAsync(stateTwo)
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
		t.Fatalf("read reply:\ngot:\n%v\nwant:\n%v", rreply, stateTwo)
	}
}

func TestManagerClose(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
		[]storageServer{
			{impl: qc.NewStorageSlow(time.Second)},
			{impl: qc.NewStorageSlow(time.Second)},
			{impl: qc.NewStorageSlow(time.Second)},
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
	err2, ok := err.(qc.QuorumCallError)
	if !ok {
		t.Fatalf("got error of type %T, want error of type %T\nerror details: %v", err2, qc.QuorumCallError{}, err)
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
		[]storageServer{
			{impl: qc.NewStorageSlowWithState(5*time.Millisecond, stateOne)},
			{impl: qc.NewStorageSlowWithState(20*time.Millisecond, stateTwo)},
			{impl: qc.NewStorageSlowWithState(500*time.Millisecond, stateTwo)},
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
	qspec := NewStorageByTimestampQSpec(majority, majority)
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
	storageServerImplementations := []*qc.StorageServerLockedWithState{
		qc.NewStorageServerLockedWithState(stateOne, 0),
		qc.NewStorageServerLockedWithState(stateTwo, 0),
		qc.NewStorageServerLockedWithState(stateTwo, 0),
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: storageServerImplementations[0]},
			{impl: storageServerImplementations[1]},
			{impl: storageServerImplementations[2]},
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
	qspec := NewStorageByTimestampQSpec(majority, majority)
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
	storageServerImplementations[0].Unlock()

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
	storageServerImplementations[1].Unlock()
	storageServerImplementations[2].Unlock()

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

	// We need the specific implementation so we call the Unlock and PerformSingleReadPrelim methods.
	storageServerImplementations := []*qc.StorageServerLockedWithState{
		qc.NewStorageServerLockedWithState(stateOne, 2),
		qc.NewStorageServerLockedWithState(stateTwo, 2),
		qc.NewStorageServerLockedWithState(stateTwo, 0),
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: storageServerImplementations[0]},
			{impl: storageServerImplementations[1]},
			{impl: storageServerImplementations[2]},
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
	config, err := mgr.NewConfiguration(ids, &ReadPrelimTestQSpec{})
	if err != nil {
		t.Fatalf("error creating config: %v", err)
	}

	ctx := context.Background()
	correctable := config.ReadPrelim(ctx, &qc.ReadRequest{})

	// We need these watchers for testing to know that a server has replied and
	// gorums has processed the reply.
	levelOneChan := correctable.Watch(1)
	levelTwoChan := correctable.Watch(2)
	levelThreeChan := correctable.Watch(3)
	levelFourChan := correctable.Watch(4)

	// Unlock all servers.
	storageServerImplementations[0].Unlock()
	storageServerImplementations[1].Unlock()
	storageServerImplementations[2].Unlock()

	// 0.1: Check that Done() is not done.
	select {
	case <-correctable.Done():
		t.Fatalf("read correctable prelim: Done() was done before any reply was received")
	default:
	}

	// 0.2: Check that Get() returns nil, LevelNotSet, nil.
	checkReplyAndLevel(t, correctable, qc.LevelNotSet, nil)
	storageServerImplementations[0].PerformSingleReadPrelim()
	// Wait for level 1 notification.
	checkLevelAndDone(t, correctable, levelOneChan, 1, false)

	// 1.2: Check that Get() returns stateOne, 1, nil.
	checkReplyAndLevel(t, correctable, 1, stateOne)
	storageServerImplementations[0].PerformSingleReadPrelim()
	// Wait for level 2 notification.
	checkLevelAndDone(t, correctable, levelTwoChan, 2, false)

	// 2.2: Check that Get() returns stateOne, 2, nil.
	checkReplyAndLevel(t, correctable, 2, stateOne)
	storageServerImplementations[1].PerformSingleReadPrelim()
	// Wait for level 3 notification.
	checkLevelAndDone(t, correctable, levelThreeChan, 3, false)

	// 3.2: Check that Get() returns stateTwo, 3, nil.
	checkReplyAndLevel(t, correctable, 3, stateTwo)
	storageServerImplementations[1].PerformSingleReadPrelim()
	// Wait for level 4 notification.
	checkLevelAndDone(t, correctable, levelFourChan, 4, true)

	// 4.2: Check that Get() returns stateTwo, 4, nil.
	checkReplyAndLevel(t, correctable, 4, stateTwo)
}

func checkLevelAndDone(t *testing.T, correctable *qc.ReadPrelimReply, levelChan <-chan struct{}, expectedLevel int, expectedDone bool) {
	t.Helper()
	waitTimeout := time.Second
	// Wait for level notification.
	select {
	case <-levelChan:
	case <-time.After(waitTimeout):
		t.Fatalf("read correctable prelim: waiting for level %d chan timed out (waited %v)", expectedLevel, waitTimeout)
	}

	if expectedDone {
		// Check that Done() is done.
		select {
		case <-correctable.Done():
		case <-time.After(waitTimeout):
			t.Fatalf("read correctable prelim: waiting for Done channel timed out (waited %v)", waitTimeout)
		}
	} else {
		// Check that Done() is not done.
		select {
		case <-correctable.Done():
			t.Fatalf("read correctable prelim: Done() was done at level %d", expectedLevel)
		default:
		}
	}
}

func checkReplyAndLevel(t *testing.T, correctable *qc.ReadPrelimReply, expectedLevel int, expectedReply *qc.State) {
	t.Helper()
	reply, level, err := correctable.Get()
	if err != nil {
		t.Fatalf("read correctable prelim: get (%d): got unexpected error: %v", expectedLevel, err)
	}
	if level != expectedLevel {
		t.Fatalf("read correctable prelim: get (%d): got level %v, want %v", expectedLevel, level, expectedLevel)
	}
	if expectedReply == nil && reply != nil {
		t.Fatalf("read correctable prelim: get (%d):\ngot reply:\n%v\nwant:\nnil", expectedLevel, reply.Value)
	}
	if reply == nil && expectedReply != nil {
		t.Fatalf("read correctable prelim: get (%d):\ngot reply:\nnil\nwant:\n%v", expectedLevel, expectedReply.Value)
	}
	if reply != nil && expectedReply != nil && reply.Value != expectedReply.Value {
		t.Fatalf("read correctable prelim: get (%d):\ngot reply:\n%v\nwant:\n%v", expectedLevel, reply.Value, expectedReply.Value)
	}
}

func TestPerNodeArg(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]storageServer{
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
			{impl: qc.NewStorageBasic()},
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
	qspec := NewStorageByTimestampQSpec(2, len(ids))

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
	if rreply.Value != state[uint32(valNodeID)].Value {
		t.Fatalf("read reply: got state %v, want state %v", rreply.Value, state[uint32(valNodeID)].Value)
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
	benchmarkRead(b, 1<<10, 1, 3, false, false, false)
}

func BenchmarkRead1KQ1N3Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, false, false, true)
}

func BenchmarkRead1KQ1N3ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, true, false, false)
}

func BenchmarkRead1KQ1N3ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, true, false, true)
}

func BenchmarkRead1KQ1N3FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, false, true, false)
}

func BenchmarkRead1KQ1N3FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, false, true, true)
}

func BenchmarkRead1KQ1N3FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, true, true, false)
}

func BenchmarkRead1KQ1N3FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 3, true, true, true)
}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KQ2N3Local(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, false, false, false)
}

func BenchmarkRead1KQ2N3Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, false, false, true)
}

func BenchmarkRead1KQ2N3ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, true, false, false)
}

func BenchmarkRead1KQ2N3ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, true, false, true)
}

func BenchmarkRead1KQ2N3FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, false, true, false)
}

func BenchmarkRead1KQ2N3FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, false, true, true)
}

func BenchmarkRead1KQ2N3FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, true, true, false)
}

func BenchmarkRead1KQ2N3FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 2, 3, true, true, true)
}

///////////////////////////////////////////////////////////////

func BenchmarkRead1KQ1N1Local(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, false, false, false)
}

func BenchmarkRead1KQ1N1Remote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, false, false, true)
}

func BenchmarkRead1KQ1N1ParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, true, false, false)
}

func BenchmarkRead1KQ1N1ParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, true, false, true)
}

func BenchmarkRead1KQ1N1FutureLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, false, true, false)
}

func BenchmarkRead1KQ1N1FutureRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, false, true, true)
}

func BenchmarkRead1KQ1N1FutureParallelLocal(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, true, true, false)
}

func BenchmarkRead1KQ1N1FutureParallelRemote(b *testing.B) {
	benchmarkRead(b, 1<<10, 1, 1, true, true, true)
}

///////////////////////////////////////////////////////////////

var replySink *qc.State

var remoteStorageHost string

func benchmarkRead(b *testing.B, psize, rq, n int, parallel, future, remote bool) {
	sservers := make([]storageServer, n)
	if remote {
		addrs := getRemoteBenchAddrs(b, n)
		for i := range sservers {
			sservers[i] = storageServer{addr: addrs[i]}
		}
	} else {
		for i := range sservers {
			sservers[i] = storageServer{impl: qc.NewStorageBench()}
		}
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(b, sservers, remote)
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
	qspec := NewStorageQSpec(rq, len(ids))
	ctx := context.Background()
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		b.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     strings.Repeat("x", psize),
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
					replySink, err = rf.Get()
					if err != nil {
						b.Fatalf("read future quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				rf := config.ReadFuture(ctx, &qc.ReadRequest{})
				replySink, err = rf.Get()
				if err != nil {
					b.Fatalf("read future quorum call error: %v", err)
				}
			}
		}
	}
}

///////////////////////////////////////////////////////////////

func BenchmarkWrite1KQ2N3Local(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, false, false, false)
}

func BenchmarkWrite1KQ2N3Remote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, false, false, true)
}

func BenchmarkWrite1KQ2N3ParallelLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, true, false, false)
}

func BenchmarkWrite1KQ2N3ParallelRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, true, false, true)
}

func BenchmarkWrite1KQ2N3FutureLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, false, true, false)
}

func BenchmarkWrite1KQ2N3FutureRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, false, true, true)
}

func BenchmarkWrite1KQ2N3FutureParallelLocal(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, true, true, false)
}

func BenchmarkWrited1KQ2N3FutureParallelRemote(b *testing.B) {
	benchmarkWrite(b, 1<<10, 2, 3, true, true, true)
}

///////////////////////////////////////////////////////////////

var wreplySink *qc.WriteResponse

func benchmarkWrite(b *testing.B, psize, wq, n int, parallel, future, remote bool) {
	sservers := make([]storageServer, n)
	if remote {
		addrs := getRemoteBenchAddrs(b, n)
		for i := range sservers {
			sservers[i] = storageServer{addr: addrs[i]}
		}
	} else {
		for i := range sservers {
			sservers[i] = storageServer{impl: qc.NewStorageBench()}
		}
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(b, sservers, remote)
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
	qspec := NewStorageQSpec(0, wq)
	ctx := context.Background()
	config, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		b.Fatalf("error creating config: %v", err)
	}

	state := &qc.State{
		Value:     strings.Repeat("x", psize),
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
					wreplySink, err = rf.Get()
					if err != nil {
						b.Fatalf("write future quorum call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				rf := config.WriteFuture(ctx, state)
				wreplySink, err = rf.Get()
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
	var rservers []storageServer
	if remote {
		addrs := getRemoteBenchAddrs(b, 1)
		rservers = []storageServer{{addr: addrs[0]}}
	} else {
		rservers = []storageServer{{impl: qc.NewStorageBench()}}
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

	rclient := qc.NewStorageClient(conn)
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

func setup(t testing.TB, storServers []storageServer, remote bool) (storageServers, qc.ManagerOption, func(n int), func(n int)) {
	if len(storServers) == 0 {
		t.Fatal("setupServers: need at least one server")
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(time.Second),
		grpc.WithInsecure(),
	}
	dialOpts := qc.WithGrpcDialOptions(grpcOpts...)

	if remote {
		return storServers, dialOpts, func(int) {}, func(int) {}
	}

	servers := make([]*grpc.Server, len(storServers))
	for i := range servers {
		servers[i] = grpc.NewServer()
		qc.RegisterStorageServer(servers[i], storServers[i].impl)
		if storServers[i].addr == "" {
			portSupplier.Lock()
			storServers[i].addr = fmt.Sprintf("localhost:%d", portSupplier.p)
			portSupplier.p++
			portSupplier.Unlock()
		}
	}

	listeners := make([]net.Listener, len(servers))

	var err error
	for i, rs := range storServers {
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
		storServers[i].addr = "localhost:" + port
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

	return storServers, dialOpts, stopGrpcServeFunc, closeListenersFunc
}

type storageServer struct {
	impl qc.StorageTestServer
	addr string
}

type storageServers []storageServer

func (rs storageServers) addrs() []string {
	addrs := make([]string, len(rs))
	for i, server := range rs {
		addrs[i] = server.addr
	}
	return addrs
}

var remoteBenchmarkHosts []string

func parseHostnames(hostnames string) error {
	if hostnames == "" {
		return nil
	}

	hostPairsSplitted := strings.Split(hostnames, ",")
	for i, hps := range hostPairsSplitted {
		tmp := strings.Split(hps, ":")
		if len(tmp) != 2 {
			return fmt.Errorf("parseHostnames: malformed host address: host %d: %q", i, hps)
		}
		remoteBenchmarkHosts = append(remoteBenchmarkHosts, hps)
	}

	return nil
}

func getRemoteBenchAddrs(tb testing.TB, n int) []string {
	tb.Helper()
	if n > len(remoteBenchmarkHosts) {
		tb.Fatalf("%d remote host(s) needed, %d provided", n, len(remoteBenchmarkHosts))
	}
	return remoteBenchmarkHosts[:n]
}

// waitForAllWrites must only be called when
// all servers in rs performs the write operation.
// Calling this method when only using a subset of servers
// is an error.
//
// TODO: Adjust this to allow waiting for a
// subset of writes.
func (rs storageServers) waitForAllWrites() {
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
