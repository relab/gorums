package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/relab/gorums/dev"

	"google.golang.org/grpc"
)

func main() {
	port := flag.String("port", "8080", "port to listen on")

	l, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	dev.RegisterRegisterServer(grpcServer, dev.NewRegisterBench())
	log.Fatal(grpcServer.Serve(l))
}
