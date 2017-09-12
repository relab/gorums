package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/relab/gorums/dev"

	"google.golang.org/grpc"
)

func main() {
	port := flag.String("port", "8080", "port to listen on")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", *port))
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	dev.RegisterStorageServer(grpcServer, dev.NewStorageBench())
	log.Fatal(grpcServer.Serve(l))
}
