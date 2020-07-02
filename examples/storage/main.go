package main

import (
	"flag"
	"strings"
)

func main() {
	server := flag.String("server", "", "Start as a server on given address.")
	remotes := flag.String("connect", "", "Comma-separated list of servers to connect to.")
	flag.Parse()

	if *server != "" {
		RunServer(*server)
		return
	}

	RunClient(strings.Split(*remotes, ","))
}
