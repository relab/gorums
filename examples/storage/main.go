package main

import (
	"flag"
	"log"
	"strings"

	"github.com/relab/gorums/examples/storage/proto"
)

func main() {
	server := flag.String("server", "", "Start as a server on given address.")
	remotes := flag.String("connect", "", "Comma-separated list of servers to connect to.")
	flag.Parse()

	if *server != "" {
		RunServer(*server)
		return
	}

	addrs := strings.Split(*remotes, ",")
	// start local servers if no remote servers were specified
	if len(addrs) == 1 && addrs[0] == "" {
		addrs = nil
		srvs := make([]*proto.GorumsServer, 0, 4)
		for i := 0; i < 4; i++ {
			srv, addr := StartServer("127.0.0.1:0")
			srvs = append(srvs, srv)
			addrs = append(addrs, addr)
			log.Printf("Started storage server on %s\n", addr)
		}
		defer func() {
			for _, srv := range srvs {
				srv.Stop()
			}
		}()
	}

	RunClient(addrs)
}
