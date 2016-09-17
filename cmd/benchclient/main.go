package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tylertreat/bench"
	"github.com/relab/gorums/gbench"
)

const (
	gorums string = "gorums"
	grpc   string = "grpc"
	byzq   string = "byzq"
)

func main() {
	var (
		mode = flag.String("mode", gorums, "mode: grpc | gorums | byzq")

		saddrs  = flag.String("addrs", ":8080,:8081,:8082", "server addresses seperated by ','")
		readq   = flag.Int("rq", 2, "read quorum size")
		writeq  = flag.Int("wq", 2, "write quorum size")
		f       = flag.Int("f", 1, "byzq fault tolerance (this is ignored if addrs is provided)")
		port    = flag.Int("port", 8080, "port where local server is listening")
		psize   = flag.Int("p", 1024, "payload size in bytes")
		timeout = flag.Duration("t", time.Second, "(Q)RPC timeout")
		writera = flag.Int("wr", 0, "write ratio in percent (0-100)")
		grpcc   = flag.Bool("grpcc", false, "run concurrent grpc")

		brrate = flag.Uint("brrate", 10000, "benchmark) request rate")
		bconns = flag.Uint("bconns", 1, "benchmark connections (separate gorums manager&config instances)")
		bdur   = flag.Duration("bdur", 30*time.Second, "benchmark duration")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	switch *mode {
	case gorums, grpc, byzq:
	default:
		dief("unknown benchmark mode: %q", *mode)
	}

	if *saddrs == "" {
		// using local addresses only
		if *f > 3 || *f < 1 {
			dief("only f=1,2,3 is allowed")
		}
		n := 3**f + 1
		var buf bytes.Buffer
		for i := 0; i < n; i++ {
			buf.WriteString(":")
			buf.WriteString(strconv.Itoa(*port + i))
			buf.WriteString(",")
		}
		b := buf.String()
		*saddrs = b[:len(b)-1]
	}

	addrs := strings.Split(*saddrs, ",")
	if len(addrs) == 0 {
		dief("no server address(es) provided")
	}
	if *writera > 100 || *writera < 0 {
		dief("invalid write ratio (%d)", *writera)
	}
	if *readq > len(addrs) || *readq < 0 {
		dief("invalid read quorum value (rq=%d, n=%d)", *readq, len(addrs))
	}
	if *writeq > len(addrs) || *writeq < 0 {
		dief("invalid write quorum value (wq=%d, n=%d)", *writeq, len(addrs))
	}

	log.SetFlags(0)
	log.SetPrefix("benchclient: ")

	var factory bench.RequesterFactory
	switch *mode {
	case gorums:
		factory = &gbench.GorumsRequesterFactory{
			Addrs:             addrs,
			ReadQuorum:        *readq,
			WriteQuorum:       *writeq,
			PayloadSize:       *psize,
			QRPCTimeout:       *timeout,
			WriteRatioPercent: *writera,
		}
	case grpc:
		factory = &gbench.GrpcRequesterFactory{
			Addrs:             addrs,
			PayloadSize:       *psize,
			Timeout:           *timeout,
			WriteRatioPercent: *writera,
			Concurrent:        *grpcc,
		}
	case byzq:
		factory = &gbench.ByzqRequesterFactory{
			Addrs:             addrs,
			PayloadSize:       *psize,
			QRPCTimeout:       *timeout,
			WriteRatioPercent: *writera,
		}
	}

	benchmark := bench.NewBenchmark(factory, uint64(*brrate), uint64(*bconns), *bdur)

	start := time.Now()
	log.Println("mode is", *mode)
	if *mode == grpc {
		log.Println("concurrent:", *grpcc)
	}
	log.Print("starting benchmark run...")
	summary, err := benchmark.Run()
	if err != nil {
		log.Fatalln("benchmark error:", err)
	}
	log.Print("done")

	benchParams := fmt.Sprintf(
		"start time: %v | #servers: %d | payload size: %d bytes | write ratio: %d%%",
		start, len(addrs), *psize, *writera,
	)
	switch *mode {
	case gorums:
		benchParams = fmt.Sprintf("%s | readq: %d", benchParams, *readq)
	case byzq:
		benchParams = fmt.Sprintf("%s | readbyz: %d", benchParams, *readq)
	}
	// if *mode == "gorums" {
	// 	benchParams = fmt.Sprintf("%s | readq: %d", benchParams, *readq)
	// }
	log.Print(benchParams)
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

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln("error opening file:", err)
	}
	defer file.Close()

	_, err = file.WriteString(benchParams)
	if err != nil {
		log.Fatalln("error writing paramterers to file:", err)
	}

	_, err = file.WriteString(summary.String())
	if err != nil {
		log.Fatalln("error writing summary to file:", err)
	}
}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}
