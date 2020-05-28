package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/relab/gorums/benchmark"
)

func main() {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [address:port]\n", os.Args[0])
		os.Exit(1)
	}

	lis, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on '%s': %v", os.Args[1], err)
		os.Exit(1)
	}
	srv := benchmark.NewServer()
	go srv.Serve(lis)

	<-signals
	fmt.Println("Exiting...")
}
