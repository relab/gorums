package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/relab/gorums/gridq"
)

var localAddrs2x2 = []string{
	":8080", ":8081",
	":8082", ":8083",
}

var localAddrs3x3 = []string{
	":8080", ":8081", ":8082",
	":8083", ":8084", ":8085",
	":8086", ":8087", ":8088",
}

func main() {
	var (
		saddrs     = flag.String("addrs", "", "server addresses separated by ','")
		srows      = flag.Int("rows", 0, "number of rows")
		predefined = flag.String("predef", "", "predefined grids ('2x2' or '3x3') for local testing")
		printGrid  = flag.Bool("gprid", true, "print quorums to screen")
		psize      = flag.Int("p", 1024, "payload size in bytes")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	var rows int
	var addrs []string
	if *predefined != "" {
		switch *predefined {
		case "2x2":
			addrs = localAddrs2x2
			rows = 2
		case "3x3":
			rows = 3
			addrs = localAddrs3x3
		default:
			dief("unknown predefined grid: %q", *predefined)
		}
	} else {
		rows = *srows
		addrs = strings.Split(*saddrs, ",")
	}

	if len(addrs) == 0 {
		dief("no server addresses provided")
	}
	if rows == 0 {
		dief("rows must be > 0")
	}
	if len(addrs)%rows != 0 {
		dief("%d addresse(s) and %d row(s) does not form a complete grid", len(addrs), rows)
	}
	cols := len(addrs) / rows

	fmt.Println("#addrs:", len(addrs), "rows:", rows, "cols:", cols)

	mgr, err := gridq.NewManager(
		addrs,
		gridq.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithInsecure(),
			grpc.WithTimeout(5*time.Second),
		),
	)
	if err != nil {
		dief("error creating manager: %v", err)
	}

	ids := mgr.NodeIDs()

	gqspec := gridq.NewGQSort(rows, cols, *printGrid)

	conf, err := mgr.NewConfiguration(ids, gqspec)
	if err != nil {
		dief("error creating config: %v", err)
	}

	for {
		state := &gridq.State{
			Value:     strings.Repeat("x", *psize),
			Timestamp: time.Now().Unix(),
		}

		fmt.Println("writing:", state)
		wreply, err := conf.Write(context.Background(), state)
		if err != nil {
			fmt.Println("error writing value:", err)
			os.Exit(2)
		}
		fmt.Println("write response:", wreply)

		time.Sleep(2 * time.Second)

		rreply, err := conf.Read(context.Background(), &gridq.Empty{})
		if err != nil {
			fmt.Println("error reading value:", err)
			os.Exit(2)
		}
		fmt.Println("read response:", rreply.State)

		time.Sleep(3 * time.Second)
	}
}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}
