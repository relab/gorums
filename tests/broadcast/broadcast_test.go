package broadcast

import (
	"context"
	fmt "fmt"
	net "net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
)

func createSrvs(numSrvs int) ([]*testServer, []string, func(), error) {
	go func() {
		http.ListenAndServe("localhost:10001", nil)
	}()
	srvs := make([]*testServer, 0, numSrvs)
	srvAddrs := make([]string, numSrvs)
	for i := 0; i < numSrvs; i++ {
		srvAddrs[i] = fmt.Sprintf("127.0.0.1:500%v", i)
	}
	for i, addr := range srvAddrs {
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
	numReqs := 5
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

	for i := 0; i < numReqs; i++ {
		val := int64(i)
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
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

func TestBroadcastCallRaceTwoClients(t *testing.T) {
	srvs, srvAddrs, srvCleanup, err := createSrvs(3)
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

	var wg sync.WaitGroup
	for i := 0; i <= 100; i++ {
		go func(j int) {
			resp, err := client1.BroadcastCall(context.Background(), &Request{Value: int64(j)})
			if err != nil {
				t.Error(err)
			}
			if resp.GetResult() != int64(j) {
				t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), j)
			}
			wg.Done()
		}(i)
		go func(j int) {
			resp, err := client1.BroadcastCall(context.Background(), &Request{Value: int64(j)})
			if err != nil {
				t.Error(err)
			}
			if resp.GetResult() != int64(j) {
				t.Errorf("result is wrong. got: %v, want: %v", resp.GetResult(), j)
			}
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}(i)
		wg.Add(2)
	}
	wg.Wait()
	for _, srv := range srvs {
		fmt.Println(srv.GetMsgs())
	}
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
		t.Fatal("resp is wrong")
	}
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
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

func BenchmarkBroadcastCall(b *testing.B) {
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
