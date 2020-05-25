package main

import (
	"fmt"
	"net"
	"os"

	"github.com/relab/gorums/benchmark"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [port to listen on]\n", os.Args[0])
	}
	lis, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on '%s': %v", os.Args[2], err)
	}
	srv := benchmark.NewServer()
	srv.Serve(lis)
}
