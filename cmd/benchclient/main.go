package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/relab/gorums/gbench"
	"github.com/tylertreat/bench"
)

func main() {
	var (
		saddrs  = flag.String("addrs", ":8080,:8081,:8082", "server addresses seperated by ','")
		readq   = flag.Int("rq", 2, "read quorum size")
		psize   = flag.Int("p", 1024, "payload size in bytes")
		timeout = flag.Duration("t", time.Second, "QRPC timeout")
		writera = flag.Int("wr", 0, "write ratio in percent (0-100)")

		brrate = flag.Uint("brrate", 10000, "benchmark request rate")
		bconns = flag.Uint("bconns", 1, "benchmark connections (separate gorums manager&config instances)")
		bdur   = flag.Duration("bdur", 30*time.Second, "benchmark duration")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	addrs := strings.Split(*saddrs, ",")
	if len(addrs) == 0 {
		dief("no server addresses provided")
	}
	if *readq > len(addrs) || *readq < 0 {
		dief("invalid read quorum value (rq=%d, n=%d)", *readq, len(addrs))
	}
	if *writera > 100 || *writera < 0 {
		dief("invalid write ratio (%d)", *writera)
	}

	log.SetFlags(0)
	log.SetPrefix("benchclient: ")

	r := &gbench.RequesterFactory{
		Addrs:             addrs,
		ReadQuorum:        *readq,
		PayloadSize:       *psize,
		QRPCTimeout:       *timeout,
		WriteRatioPercent: *writera,
	}

	benchmark := bench.NewBenchmark(r, uint64(*brrate), uint64(*bconns), *bdur)
	start := time.Now()
	log.Print("starting benchmark run...")
	summary, err := benchmark.Run()
	if err != nil {
		log.Fatalln("benchmark error:", err)
	}
	log.Print("done")

	log.Printf(
		"start time: %v | #servers: %d | read quorum: %d | payload size: %d bytes | write ratio: %d%%",
		start, len(addrs), *readq, *psize, *writera,
	)
	log.Println("summary:", summary)

	filename := fmt.Sprintf(
		"gorums-%04d%02d%02d-%02d%02d%02d.txt",
		start.Year(), start.Month(), start.Day(),
		start.Hour(), start.Minute(), start.Second(),
	)
	err = summary.GenerateLatencyDistribution(bench.Logarithmic, filename)
	if err != nil {
		log.Printf("error writing latency distribution to file: %v", err)
	}
}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}
