package broadcast

import (
	"context"
	fmt "fmt"
	net "net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

func createSrvs(numSrvs int, down ...int) ([]*testServer, []string, func(), error) {
	skip := 0
	if len(down) > 0 {
		skip = down[0]
	}
	go func() {
		http.ListenAndServe("localhost:10001", nil)
	}()
	srvs := make([]*testServer, 0, numSrvs)
	srvAddrs := make([]string, numSrvs)
	for i := 0; i < numSrvs; i++ {
		srvAddrs[i] = fmt.Sprintf("127.0.0.1:500%v", i)
	}
	for i, addr := range srvAddrs {
		if skip > 0 {
			skip--
			continue
		}
		srv := newtestServer(addr, srvAddrs, i)
		lis, err := net.Listen("tcp4", srv.addr)
		if err != nil {
			return nil, nil, nil, err
		}
		srv.lis = lis
		go srv.start(lis)
		srvs = append(srvs, srv)
	}
	return srvs, srvAddrs, func() {
		// stop the servers
		for _, srv := range srvs {
			srv.Stop()
		}
	}, nil
}

func TestSimpleBroadcastCall(t *testing.T) {
	numSrvs := 3
	numReqs := 10
	srvs, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 1; i <= numReqs; i++ {
		//slog.Info("req", "no", i)
		val := int64(i * 100)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := config.BroadcastCall(ctx, &Request{Value: val})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != val {
			t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", val, resp.GetResult()))
		}
		cancel()
	}
	for _, srv := range srvs {
		if srv.GetNumMsgs() != numReqs*numSrvs {
			//t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", numReqs*numSrvs, srv.GetNumMsgs()))
		}
	}
}

func TestSimpleBroadcastTo(t *testing.T) {
	numSrvs := 3
	numReqs := 10
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	// only want a response from the leader, hence qsize = 1
	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080", 1)
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 1; i <= numReqs; i++ {
		//slog.Info("req", "no", i)
		val := int64(i * 100)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := config.BroadcastCallTo(ctx, &Request{Value: val})
		if err != nil {
			t.Error(err)
		}
		if resp.GetFrom() != leader {
			t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", leader, resp.GetFrom()))
		}
		cancel()
	}
}

func TestSimpleBroadcastCancel(t *testing.T) {
	numSrvs := 3
	numReqs := 10
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	// only want a response from the leader, hence qsize = 1
	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 1; i <= numReqs; i++ {
		//slog.Info("req", "no", i)
		val := int64(i * 100)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := config.Search(ctx, &Request{Value: val})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != 1 {
			t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", 1, resp.GetResult()))
		}
		cancel()
	}
}

func TestBroadcastCancel(t *testing.T) {
	numSrvs := 3
	numReqs := 10
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 1; i <= numReqs; i++ {
		val := int64(i * 100)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		config.LongRunningTask(ctx, &Request{Value: val})
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := config.GetVal(ctx, &Request{Value: val})
		cancel()
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != 1 {
			t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", 1, resp.GetResult()))
		}
	}
}

func TestBroadcastCancelOneSrvDown(t *testing.T) {
	numSrvs := 3
	numReqs := 10
	srvs, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	// stop one of the servers
	srvs[numSrvs-1].Stop()
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	// only want response from the online servers
	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080", numSrvs-1)
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 1; i <= numReqs; i++ {
		val := int64(i * 100)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		config.LongRunningTask(ctx, &Request{Value: val})
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := config.GetVal(ctx, &Request{Value: val})
		cancel()
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != 1 {
			t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", 1, resp.GetResult()))
		}
	}
}

func TestBroadcastCancelOneSrvFails(t *testing.T) {
	numSrvs := 3
	srvs, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	// only want response from the online servers
	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080", numSrvs-1)
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(100)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	config.LongRunningTask(ctx, &Request{Value: val})
	cancel()

	// stop one of the servers
	srvs[numSrvs-1].Stop()

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := config.GetVal(ctx, &Request{Value: val})
	cancel()
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != 1 {
		t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", 1, resp.GetResult()))
	}
}

func TestBroadcastCancelOneClientFails(t *testing.T) {
	numSrvs := 3
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	// only want response from the online servers
	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}

	val := int64(100)
	go config.LongRunningTask(context.Background(), &Request{Value: val})

	// make sure the request is sent and stop the client
	time.Sleep(100 * time.Millisecond)
	clientCleanup()

	// only want response from the online servers
	config2, clientCleanup2, err2 := newClient(srvAddrs, "127.0.0.1:8081")
	defer clientCleanup2()
	if err2 != nil {
		t.Error(err2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := config2.GetVal(ctx, &Request{Value: val})
	cancel()
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != 1 {
		t.Error(fmt.Sprintf("resp is wrong, want: %v, got: %v", 1, resp.GetResult()))
	}
}

func TestBroadcastCallRace(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
	for i := 0; i <= 100; i++ {
		resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
	}
}

func TestBroadcastCallClientKnowsOnlyOneServer(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs[0:1], "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
	for i := 0; i <= 100; i++ {
		resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
	}
}

func TestBroadcastCallOneServerIsDown(t *testing.T) {
	numSrvs := 3
	skip := 1
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs, skip)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	start := skip
	if start < 0 {
		start = 0
	}
	end := numSrvs - 1
	if end > len(srvAddrs) {
		end = len(srvAddrs)
	}
	config, clientCleanup, err := newClient(srvAddrs[start:end], "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
	for i := 0; i <= 100; i++ {
		resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
	}
}

func TestBroadcastCallForward(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs[1:2], "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		resp, err := config.BroadcastCallForward(ctx, &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
	}
}

func TestBroadcastCallForwardMultiple(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs[1:], "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	for i := 0; i < 10; i++ {
		resp, err := config.BroadcastCallForward(context.Background(), &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
	}
}

func TestBroadcastCallRaceTwoClients(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	client1, clientCleanup1, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup1()

	client2, clientCleanup2, err := newClient(srvAddrs, "127.0.0.1:8081")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup2()

	val := int64(1)
	resp, err := client1.BroadcastCall(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
	resp, err = client2.BroadcastCall(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}

	timeout := 3 * time.Second
	var wg sync.WaitGroup
	for i := 0; i <= 100; i++ {
		go func(j int) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			resp, err := client1.BroadcastCall(ctx, &Request{Value: int64(j)})
			if err != nil {
				t.Error(err)
			}
			if resp.GetResult() != int64(j) {
				t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), j)
			}
			wg.Done()
		}(i)
		go func(j int) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			resp, err := client2.BroadcastCall(ctx, &Request{Value: int64(j)})
			if err != nil {
				t.Error(err)
			}
			if resp.GetResult() != int64(j) {
				t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), j)
			}
			wg.Done()
		}(i)
		wg.Add(2)
	}
	wg.Wait()
}

func TestBroadcastCallAsyncReqs(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	numClients := 10
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs, fmt.Sprintf("127.0.0.1:808%v", c), 3)
		if err != nil {
			t.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(init) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 1; i++ {
		for _, client := range clients {
			go func(j int, c *Configuration) {
				resp, err := c.BroadcastCall(context.Background(), &Request{Value: int64(j)})
				if err != nil {
					t.Error(err)
				}
				if resp.GetResult() != int64(j) {
					t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), j)
				}
				wg.Done()
			}(i, client)
			wg.Add(1)
		}
	}
	wg.Wait()
}

func TestQCBroadcastOptionRace(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	resp, err := config.QuorumCallWithBroadcast(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatalf("resp is wrong, got: %v, want: %v", resp.GetResult(), val)
	}
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := config.QuorumCallWithBroadcast(ctx, &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
		cancel()
	}
}

func TestQCMulticastRace(t *testing.T) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		t.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		t.Error(err)
	}
	defer clientCleanup()

	val := int64(1)
	resp, err := config.QuorumCallWithMulticast(context.Background(), &Request{Value: val})
	if err != nil {
		t.Error(err)
	}
	if resp.GetResult() != val {
		t.Fatal("resp is wrong")
	}
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		resp, err := config.QuorumCallWithMulticast(ctx, &Request{Value: int64(i)})
		if err != nil {
			t.Error(err)
		}
		if resp.GetResult() != int64(i) {
			t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
		}
		cancel()
	}
}

func BenchmarkQuorumCall(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "")
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.QuorumCall(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileQF")
	memProfile, _ := os.Create("memprofileQF")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("QC_AllSuccessful_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.QuorumCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	/*b.Run(fmt.Sprintf("QC_SomeFailing_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.QuorumCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
			//slog.Warn("client reply", "val", resp.GetResult())
		}
	})*/

	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkQCMulticast(b *testing.B) {
	// go test -bench=BenchmarkBroadcastOption -benchmem -count=5 -run=^# -benchtime=5x
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "")
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.QuorumCallWithMulticast(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileQFwithB")
	memProfile, _ := os.Create("memprofileQFwithB")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("QCM_AllSuccessful_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			resp, err := config.QuorumCallWithMulticast(ctx, &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
			cancel()
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkQCBroadcastOption(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "")
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.QuorumCallWithBroadcast(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileQFwithB")
	memProfile, _ := os.Create("memprofileQFwithB")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("QCB_AllSuccessful_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			resp, err := config.QuorumCallWithBroadcast(ctx, &Request{Value: int64(i)})
			if err != nil {
				b.Error(err, i)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
			cancel()
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkQCBroadcastOptionManyClients(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	numClients := 10
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs, fmt.Sprintf("127.0.0.1:808%v", c), 3)
		if err != nil {
			b.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.QuorumCallWithBroadcast(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			b.Error(err)
		}
		if resp.GetResult() != int64(init) {
			b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	cpuProfile, _ := os.Create("cpuprofileQCB")
	memProfile, _ := os.Create("memprofileQCB")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("QCB_ManyClients_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for _, client := range clients {
				go func(i int, c *Configuration) {
					resp, err := c.QuorumCallWithBroadcast(context.Background(), &Request{Value: int64(i)})
					if err != nil {
						b.Error(err)
					}
					if resp.GetResult() != int64(i) {
						b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
					}
					wg.Done()
				}(i, client)
				wg.Add(1)
			}
			wg.Wait()
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkBroadcastCallAllServers(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080")
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileBC")
	memProfile, _ := os.Create("memprofileBC")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("BC_AllSuccessful_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkBroadcastCallToOneServer(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs[0:1], "127.0.0.1:8080", 3)
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileBC")
	memProfile, _ := os.Create("memprofileBC")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("BC_OneSrv_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkBroadcastCallOneFailedServer(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3, 1)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs, "127.0.0.1:8080", 2)
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileBC")
	memProfile, _ := os.Create("memprofileBC")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("BC_OneSrvDown_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkBroadcastCallOneDownSrvToOneSrv(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3, 1)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	config, clientCleanup, err := newClient(srvAddrs[1:2], "127.0.0.1:8080", 2)
	if err != nil {
		b.Error(err)
	}
	defer clientCleanup()

	init := 1
	resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(init)})
	if err != nil {
		b.Error(err)
	}
	if resp.GetResult() != int64(init) {
		b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
	}

	cpuProfile, _ := os.Create("cpuprofileBC")
	memProfile, _ := os.Create("memprofileBC")
	pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("BC_OneDownToOne_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := config.BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	pprof.StopCPUProfile()
	pprof.WriteHeapProfile(memProfile)
}

func BenchmarkBroadcastCallManyClients(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	numClients := 10
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs[0:1], fmt.Sprintf("127.0.0.1:%v", 8080+c), 3)
		if err != nil {
			b.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			b.Error(err)
		}
		if resp.GetResult() != int64(init) {
			b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	//stop, err := StartTrace("traceprofileBC")
	//if err != nil {
	//b.Error(err)
	//}
	//defer stop()
	//cpuProfile, _ := os.Create("cpuprofileBC")
	//memProfile, _ := os.Create("memprofileBC")
	//runtime.GC()
	//pprof.StartCPUProfile(cpuProfile)

	b.Run(fmt.Sprintf("BC_OneClientOneReq_%d", 0), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			resp, err := clients[0].BroadcastCall(context.Background(), &Request{Value: int64(i)})
			if err != nil {
				b.Error(err)
			}
			if resp.GetResult() != int64(i) {
				b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
			}
		}
	})
	b.Run(fmt.Sprintf("BC_OneClientAsync_%d", 1), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for j := range clients {
				go func(i, j int, c *Configuration) {
					val := i*100 + j
					resp, err := c.BroadcastCall(context.Background(), &Request{Value: int64(val)})
					if err != nil {
						b.Error(err)
					}
					if resp.GetResult() != int64(val) {
						b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), val)
					}
					wg.Done()
				}(i, j, clients[0])
				wg.Add(1)
			}
			wg.Wait()
		}
	})
	b.Run(fmt.Sprintf("BC_OneClientSync_%d", 2), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := range clients {
				val := i*100 + j
				resp, err := clients[0].BroadcastCall(context.Background(), &Request{Value: int64(val)})
				if err != nil {
					b.Error(err)
				}
				if resp.GetResult() != int64(val) {
					b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), val)
				}
			}
		}
	})
	b.Run(fmt.Sprintf("BC_ManyClientsAsync_%d", 3), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for _, client := range clients {
				go func(i int, c *Configuration) {
					resp, err := c.BroadcastCall(context.Background(), &Request{Value: int64(i)})
					if err != nil {
						b.Error(err)
					}
					if resp.GetResult() != int64(i) {
						b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
					}
					wg.Done()
				}(i, client)
				wg.Add(1)
			}
			wg.Wait()
		}
	})
	b.Run(fmt.Sprintf("BC_ManyClientsSync_%d", 4), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, client := range clients {
				resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(i)})
				if err != nil {
					b.Error(err)
				}
				if resp.GetResult() != int64(i) {
					b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
				}
			}
		}
	})
	//pprof.StopCPUProfile()
	//pprof.WriteHeapProfile(memProfile)
	//cpuProfile.Close()
	//memProfile.Close()
}

func BenchmarkBroadcastCallTenClientsCPU(b *testing.B) {
	_, srvAddrs, srvCleanup, err := createSrvs(3)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	numClients := 10
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs[0:1], fmt.Sprintf("127.0.0.1:%v", 8080+c), 3)
		if err != nil {
			b.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			b.Error(err)
		}
		if resp.GetResult() != int64(init) {
			b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	cpuProfile, _ := os.Create("cpuprofileBC")
	err = pprof.StartCPUProfile(cpuProfile)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	b.Run(fmt.Sprintf("BC_%d", 3), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for _, client := range clients {
				go func(i int, c *Configuration) {
					resp, err := c.BroadcastCall(context.Background(), &Request{Value: int64(i)})
					if err != nil {
						b.Error(err)
					}
					if resp.GetResult() != int64(i) {
						b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
					}
					wg.Done()
				}(i, client)
				wg.Add(1)
			}
			wg.Wait()
		}
	})
	b.StopTimer()
	pprof.StopCPUProfile()
	cpuProfile.Close()
}

func BenchmarkBroadcastCallTenClientsMEM(b *testing.B) {
	numSrvs := 3
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	numClients := 1
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs[0:1], fmt.Sprintf("127.0.0.1:%v", 8080+c), numSrvs)
		if err != nil {
			b.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			b.Error(err)
		}
		if resp.GetResult() != int64(init) {
			b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	memProfile, _ := os.Create("memprofileBC")
	runtime.GC()

	b.ResetTimer()
	for _, client := range clients {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				resp, err := client.BroadcastCall(ctx, &Request{Value: int64(i)})
				cancel()
				if err != nil {
					b.Error(err)
				}
				if resp.GetResult() != int64(i) {
					b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
				}
			}
		})
	}
	b.StopTimer()
	pprof.WriteHeapProfile(memProfile)
	memProfile.Close()
}

func BenchmarkBroadcastCallTenClientsTRACE(b *testing.B) {
	numSrvs := 1
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		b.Error(err)
	}
	defer srvCleanup()

	numClients := 1
	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs[0:1], fmt.Sprintf("127.0.0.1:%v", 8080+c), numSrvs)
		if err != nil {
			b.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	for _, client := range clients {
		init := 1
		resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
		if err != nil {
			b.Error(err)
		}
		if resp.GetResult() != int64(init) {
			b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
		}
	}

	stop, err := StartTrace("traceprofileBC")
	if err != nil {
		b.Error(err)
	}
	defer stop()

	b.ResetTimer()
	for _, client := range clients {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				resp, err := client.BroadcastCall(ctx, &Request{Value: int64(i)})
				cancel()
				if err != nil {
					b.Error(err)
				}
				if resp.GetResult() != int64(i) {
					b.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
				}
			}
		})
	}
	b.StopTimer()
}

func TestBroadcastCallTenClientsOnlyAsync(t *testing.T) {
	numSrvs := 3
	numClients := 20
	numReqs := 500
	_, srvAddrs, srvCleanup, err := createSrvs(numSrvs)
	if err != nil {
		t.Error(err)
	}

	clients := make([]*Configuration, numClients)
	for c := 0; c < numClients; c++ {
		config, clientCleanup, err := newClient(srvAddrs[0:1], fmt.Sprintf("127.0.0.1:%v", 8080+c), numSrvs)
		if err != nil {
			t.Error(err)
		}
		defer clientCleanup()
		clients[c] = config
	}

	var wg1 sync.WaitGroup
	for _, client := range clients {
		init := 1
		go func(client *Configuration) {
			resp, err := client.BroadcastCall(context.Background(), &Request{Value: int64(init)})
			if err != nil {
				t.Error(err)
			}
			if resp.GetResult() != int64(init) {
				t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), init)
			}
			wg1.Done()
		}(client)
		wg1.Add(1)
	}
	wg1.Wait()
	time.Sleep(1 * time.Second)

	var wg sync.WaitGroup
	for r := 0; r < numReqs; r++ {
		for i, client := range clients {
			go func(i int) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				resp, err := client.BroadcastCall(ctx, &Request{Value: int64(i)})
				if err != nil {
					t.Error(err)
				}
				if resp.GetResult() != int64(i) {
					t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), i)
				}
				wg.Done()
			}(i)
			wg.Add(1)
		}
	}
	wg.Wait()
	srvCleanup()
}
