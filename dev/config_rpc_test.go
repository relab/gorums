package dev_test

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	rpc "github.com/relab/gorums/dev"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
)

func TestMain(m *testing.M) {
	flag.Parse()
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	res := m.Run()
	os.Exit(res)
}

func TestBasicRegister(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterBasic()},
			{":8081", rpc.NewRegisterBasic()},
			{":8082", rpc.NewRegisterBasic()},
		},
		false,
	)
	defer stopGrpcServe(allServers)

	// Specify a custom quorum and picker function for the Read RPC method.
	// The user may, as an example, want to sort the replies based on their
	// timestamp.
	rqfn := func(c *rpc.Configuration, replies []*rpc.State) (*rpc.State, bool) {
		if len(replies) < c.Quorum() {
			return nil, false
		}
		sort.Sort(ByTimestamp(replies))
		return replies[len(replies)-1], true
	}

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
		rpc.WithReadQuorumFunc(rqfn),
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// A read configuration. Quorum=1, n=3
	readConfig, err := mgr.NewConfiguration(ids, 1, time.Second)
	if err != nil {
		t.Fatalf("error creating read config: %v", err)
	}

	// A write configuration. Quorum=2, n=3
	writeConfig, err := mgr.NewConfiguration(ids, 2, time.Second)
	if err != nil {
		t.Fatalf("error creating write config: %v", err)
	}

	// Test state
	state := &rpc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	// Do write call
	wreply, err := writeConfig.Write(state)
	if err != nil {
		t.Fatalf("write rpc call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.Reply.New {
		t.Error("write reply was not marked as new")
	}

	// Do read call
	rreply, err := readConfig.Read(&rpc.ReadRequest{})
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

func TestExitHandleRepliesLoop(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterBasic()},
			{":8081", rpc.NewRegisterBasic()},
			{":8082", rpc.NewRegisterBasic()},
		},
		false,
	)
	defer stopGrpcServe(allServers)

	// Read quorum picker that never returns true.
	rqfn := func(c *rpc.Configuration, replies []*rpc.State) (*rpc.State, bool) {
		return nil, false
	}

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
		rpc.WithReadQuorumFunc(rqfn),
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	ids := mgr.NodeIDs()

	readConfig, err := mgr.NewConfiguration(ids, 2, time.Second)
	if err != nil {
		t.Fatalf("error creating read config: %v", err)
	}

	replyChan := make(chan error, 1)
	go func() {
		_, err = readConfig.Read(&rpc.ReadRequest{})
		replyChan <- err
	}()
	select {
	case err := <-replyChan:
		if err == nil {
			t.Errorf("want error, got none")
		}
		incompErr, ok := err.(rpc.IncompleteRPCError)
		if !ok {
			t.Errorf("got error of type %T, want error of type %T",
				err, rpc.IncompleteRPCError{})
		}
		wantErr := rpc.IncompleteRPCError{ErrCount: 0, ReplyCount: 3}
		if incompErr != wantErr {
			t.Errorf("got: %v, want: %v", incompErr, wantErr)
		}
	case <-time.After(time.Second):
		t.Errorf("read rpc call: timeout, call did not return")
	}
}

func TestSlowRegister(t *testing.T) {
	defer leakCheck(t)()
	someErr := grpc.Errorf(codes.Unknown, "Some error")
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterSlow(time.Second)},
			{":8081", rpc.NewRegisterSlow(time.Second)},
			{":8082", rpc.NewRegisterError(someErr)},
			// Q=2 below, one error server, two slow servers
			// -> must timeout with one error received.
		},
		false,
	)
	defer stopGrpcServe(allServers)

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	ids := mgr.NodeIDs()
	timeout := 25 * time.Millisecond

	readConfig, err := mgr.NewConfiguration(ids, 2, timeout)
	if err != nil {
		t.Fatalf("error creating read config: %v", err)
	}

	_, err = readConfig.Read(&rpc.ReadRequest{})
	if err == nil {
		t.Errorf("read rpc call: want error, got none")
	}
	timeoutErr, ok := err.(rpc.TimeoutRPCError)
	if !ok {
		t.Errorf("got error of type %T, want error of type %T", err, rpc.TimeoutRPCError{})
	}
	wantErr := rpc.TimeoutRPCError{Waited: timeout, ErrCount: 1, RepliesCount: 0}
	if timeoutErr != wantErr {
		t.Errorf("got: %v, want: %v", timeoutErr, wantErr)
	}
}

func TestBasicRegisterUsingFuture(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterBasic()},
			{":8081", rpc.NewRegisterBasic()},
			{":8082", rpc.NewRegisterBasic()},
		},
		false,
	)
	defer stopGrpcServe(allServers)

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	ids := mgr.NodeIDs()

	readConfig, err := mgr.NewConfiguration(ids, 1, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("error creating read config: %v", err)
	}

	writeConfig, err := mgr.NewConfiguration(ids, 2, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("error creating write config: %v", err)
	}

	state := &rpc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	// Write asynchronously.
	wfuture := writeConfig.WriteFuture(state)

	// Wait for all writes to finish.
	servers.waitForAllWrites()

	wreply, err := wfuture.Get()
	if err != nil {
		t.Fatalf("write future rpc call error: %v", err)
	}
	t.Logf("wreply: %v\n", wreply)
	if !wreply.Reply.New {
		t.Error("write future reply was not marked as new")
	}

	// Done should report true.
	done := wfuture.Done()
	if !done {
		t.Fatalf("write future was not done")
	}

	// Read asynchronously.
	rfuture := readConfig.ReadFuture(&rpc.ReadRequest{})

	// Inspect read reply when available.
	rreply, err := rfuture.Get()
	if err != nil {
		t.Fatalf("read future rpc call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Reply.Value != state.Value {
		t.Errorf("read future reply:\nwant:\n%v,\ngot:\n%v", state, rreply.Reply)
	}
}

func TestBasicRegisterWithWriteAsync(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterBasic()},
			{":8081", rpc.NewRegisterBasic()},
			{":8082", rpc.NewRegisterBasic()},
		},
		false,
	)
	defer stopGrpcServe(allServers)

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	ids := mgr.NodeIDs()

	readConfig, err := mgr.NewConfiguration(ids, 1, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("error creating read config: %v", err)
	}

	writeConfig, err := mgr.NewConfiguration(ids, 2, 25*time.Millisecond)
	if err != nil {
		t.Fatalf("error creating write config: %v", err)
	}

	stateOne := &rpc.State{
		Value:     "42",
		Timestamp: time.Now().UnixNano(),
	}

	wreply, err := writeConfig.Write(stateOne)
	if err != nil {
		t.Fatalf("write rpc call error: %v", err)
	}

	// Drain all writers after synchronous call.
	servers.waitForAllWrites()

	t.Logf("wreply: %v\n", wreply)
	if !wreply.Reply.New {
		t.Error("write reply was not marked as new")
	}

	rreply, err := readConfig.Read(&rpc.ReadRequest{})
	if err != nil {
		t.Fatalf("read rpc call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Reply.Value != stateOne.Value {
		t.Errorf("read reply:\nwant:\n%v,\ngot:\n%v", stateOne, rreply.Reply)
	}

	stateTwo := &rpc.State{
		Value:     "99",
		Timestamp: time.Now().UnixNano(),
	}

	// Write a value using the WriteAsync stream.
	err = writeConfig.WriteAsync(stateTwo)
	if err != nil {
		t.Fatalf("write-async rpc call error: %v", err)
	}

	// Wait for all writes to finish.
	servers.waitForAllWrites()

	rreply, err = readConfig.Read(&rpc.ReadRequest{})
	if err != nil {
		t.Fatalf("read rpc call error: %v", err)
	}
	t.Logf("rreply: %v\n", rreply)
	if rreply.Reply.Value != stateTwo.Value {
		t.Errorf("read reply:\nwant:\n%v\ngot:\n%v", stateTwo, rreply.Reply)
	}
}

func TestManagerClose(t *testing.T) {
	defer leakCheck(t)()
	servers, dialOpts, stopGrpcServe, closeListeners := setup(
		t,
		[]regServer{
			{":8080", rpc.NewRegisterBasic()},
			{":8081", rpc.NewRegisterBasic()},
			{":8082", rpc.NewRegisterBasic()},
		},
		false,
	)
	defer stopGrpcServe(allServers)

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		t.Fatalf("%v", err)
	}
	closeListeners(allServers)

	const timeoutDur = time.Second
	closeReturnedChan := make(chan struct{}, 1)

	go func() {
		mgr.Close()
		close(closeReturnedChan)
	}()

	select {
	case <-closeReturnedChan:
	case <-time.After(timeoutDur):
		t.Errorf("mgr.Close() timed out, waited %v", timeoutDur)
	}
}

func BenchmarkRead1KLocal(b *testing.B)                { benchmarkRead(b, 1<<10, false, false, false) }
func BenchmarkRead1KRemote(b *testing.B)               { benchmarkRead(b, 1<<10, false, false, true) }
func BenchmarkRead1KParallelLocal(b *testing.B)        { benchmarkRead(b, 1<<10, true, false, false) }
func BenchmarkRead1KParallelRemote(b *testing.B)       { benchmarkRead(b, 1<<10, true, false, true) }
func BenchmarkRead1KFutureLocal(b *testing.B)          { benchmarkRead(b, 1<<10, false, true, false) }
func BenchmarkRead1KFutureRemote(b *testing.B)         { benchmarkRead(b, 1<<10, false, true, true) }
func BenchmarkRead1KFutureParallelLocal(b *testing.B)  { benchmarkRead(b, 1<<10, true, true, false) }
func BenchmarkRead1KFutureParallelRemote(b *testing.B) { benchmarkRead(b, 1<<10, true, true, true) }

func BenchmarkRead64KLocal(b *testing.B)                { benchmarkRead(b, 1<<16, false, false, false) }
func BenchmarkRead64KRemote(b *testing.B)               { benchmarkRead(b, 1<<16, false, false, true) }
func BenchmarkRead64KParallelLocal(b *testing.B)        { benchmarkRead(b, 1<<16, true, false, false) }
func BenchmarkRead64KParallelRemote(b *testing.B)       { benchmarkRead(b, 1<<16, true, false, true) }
func BenchmarkRead64KFutureLocal(b *testing.B)          { benchmarkRead(b, 1<<16, false, true, false) }
func BenchmarkRead64KFutureRemote(b *testing.B)         { benchmarkRead(b, 1<<16, false, true, true) }
func BenchmarkRead64KFutureParallelLocal(b *testing.B)  { benchmarkRead(b, 1<<16, true, true, false) }
func BenchmarkRead64KFutureParallelRemote(b *testing.B) { benchmarkRead(b, 1<<16, true, true, true) }

var replySink *rpc.ReadReply

func benchmarkRead(b *testing.B, size int, parallel, future, remote bool) {
	var rservers []regServer
	if !remote {
		rservers = []regServer{
			{":8080", rpc.NewRegisterBench()},
			{":8081", rpc.NewRegisterBench()},
			{":8082", rpc.NewRegisterBench()},
		}
	} else {
		rservers = []regServer{
			{"pitter31:8080", nil},
			{"pitter32:8080", nil},
			{"pitter33:8080", nil},
		}
	}

	servers, dialOpts, stopGrpcServe, closeListeners := setup(b, rservers, remote)
	defer stopGrpcServe(allServers)

	timeout := 50 * time.Millisecond
	if remote {
		timeout = timeout * 20 // 1000 ms
	}

	mgr, err := rpc.NewManager(
		servers.addrs(),
		dialOpts,
	)
	if err != nil {
		b.Fatalf("%v", err)
	}
	defer mgr.Close()
	closeListeners(allServers)

	ids := mgr.NodeIDs()

	readConfig, err := mgr.NewConfiguration(ids, 1, timeout)
	if err != nil {
		b.Fatalf("error creating read config: %v", err)
	}

	writeConfig, err := mgr.NewConfiguration(ids, 3, timeout)
	if err != nil {
		b.Fatalf("error creating write config: %v", err)
	}

	state := &rpc.State{
		Value:     strings.Repeat("x", size),
		Timestamp: time.Now().UnixNano(),
	}

	wreply, err := writeConfig.Write(state)
	if err != nil {
		b.Fatalf("write rpc call error: %v", err)
	}
	if !wreply.Reply.New {
		b.Fatalf("intital write reply was not marked as new")
	}

	b.SetBytes(int64(state.Size()))
	b.ReportAllocs()
	b.ResetTimer()

	if !future {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					replySink, err = readConfig.Read(&rpc.ReadRequest{})
					if err != nil {
						b.Fatalf("read rpc call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				replySink, err = readConfig.Read(&rpc.ReadRequest{})
				if err != nil {
					b.Fatalf("read rpc call error: %v", err)
				}
			}
		}
	} else {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					rf := readConfig.ReadFuture(&rpc.ReadRequest{})
					replySink, err = rf.Get()
					if err != nil {
						b.Fatalf("read future rpc call error: %v", err)
					}
				}
			})
		} else {
			for i := 0; i < b.N; i++ {
				rf := readConfig.ReadFuture(&rpc.ReadRequest{})
				replySink, err = rf.Get()
				if err != nil {
					b.Fatalf("read future rpc call error: %v", err)
				}
			}
		}
	}
}

const allServers = -1

func setup(t testing.TB, regServers []regServer, remote bool) (regServers, rpc.ManagerOption, func(n int), func(n int)) {
	if len(regServers) == 0 {
		t.Fatal("setupServers: need at least one server")
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(50 * time.Millisecond),
		grpc.WithInsecure(),
	}
	dialOpts := rpc.WithGrpcDialOptions(grpcOpts...)

	if remote {
		return regServers, dialOpts, func(int) {}, func(int) {}
	}

	servers := make([]*grpc.Server, len(regServers))
	for i := range servers {
		servers[i] = grpc.NewServer()
	}
	for i, server := range servers {
		rpc.RegisterRegisterServer(server, regServers[i].implementation)
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
	addr           string
	implementation rpc.RegisterTestServer
}

type regServers []regServer

func (rs regServers) addrs() []string {
	addrs := make([]string, len(rs))
	for i, server := range rs {
		addrs[i] = server.addr
	}
	return addrs
}

func (rs regServers) waitForAllWrites() {
	for _, server := range rs {
		server.implementation.WriteExecuted()
	}
}

type ByTimestamp []*rpc.State

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
		if strings.HasPrefix(stack, "testing.RunTests") {
			continue
		}

		if stack == "" ||
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
