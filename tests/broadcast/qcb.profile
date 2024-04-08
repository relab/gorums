goos: linux
goarch: amd64
pkg: github.com/relab/gorums/tests/broadcast
cpu: 11th Gen Intel(R) Core(TM) i5-11300H @ 3.10GHz
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    158939 ns/op	   40004 B/op	     707 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    158950 ns/op	   39855 B/op	     707 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    159995 ns/op	   40923 B/op	     708 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    159867 ns/op	   38821 B/op	     707 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    159868 ns/op	   39011 B/op	     708 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    166392 ns/op	   43033 B/op	     708 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    165096 ns/op	   38847 B/op	     707 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    158566 ns/op	   38821 B/op	     707 allocs/op
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	--- FAIL: BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8
    broadcast_test.go:374: quorum call error: context deadline exceeded (errors: 0, replies: 2) 3141
    broadcast_test.go:377: result is wrong. got: 0, want: 3141
BenchmarkQCBroadcastOption/QCB_AllSuccessful_1-8         	    5000	    162302 ns/op	   39042 B/op	     708 allocs/op
PASS
ok  	github.com/relab/gorums/tests/broadcast	9.015s
