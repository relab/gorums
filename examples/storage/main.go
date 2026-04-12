package main

import (
	"flag"
	"log"
	"strings"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/interceptors"
	pb "github.com/relab/gorums/examples/storage/proto"
)

func main() {
	cluster := flag.Bool("cluster", false, "Spawn a server process for each address in -addrs.")
	serve := flag.Bool("serve", false, "Run as a single server node on addrs[0]; the remaining addresses are peers.")
	addrs := flag.String("addrs", "", "Comma-separated list of server addresses.")
	ic := flag.String("interceptors", "", "Comma-separated list of interceptors to enable (logging, nofoo, metadata, delayed).")
	flag.Parse()

	if *cluster {
		if *addrs == "" {
			log.Fatal("Usage: storage -cluster -addrs <addr>[,<addr>...]")
		}
		if err := runCluster(*addrs, *ic); err != nil {
			log.Fatal(err)
		}
		return
	}

	all := splitAddrs(*addrs)
	if *serve {
		if len(all) == 0 {
			log.Fatal("Usage: storage -serve -addrs <myaddr>[,<peer>...]")
		}
		if err := runServer(all[0], all, parseInterceptors(*ic)); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Client mode: connect to the provided addresses, or spin up a local cluster (same process).
	if len(all) == 0 {
		clusterAddrs, stop, err := runLocalCluster(parseInterceptors(*ic))
		if err != nil {
			log.Fatal(err)
		}
		defer stop()
		all = clusterAddrs
	}
	if err := runClient(all); err != nil {
		log.Fatal(err)
	}
}

// splitAddrs splits a comma-separated address string, trimming spaces.
// Returns nil if s is empty.
func splitAddrs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// parseInterceptors converts a comma-separated interceptor list into server options.
func parseInterceptors(ic string) gorums.ServerOption {
	if ic == "" {
		return nil
	}
	var ics []gorums.Interceptor
	for name := range strings.SplitSeq(ic, ",") {
		switch strings.TrimSpace(name) {
		case "logging":
			ics = append(ics, interceptors.LoggingSimpleInterceptor)
		case "nofoo":
			ics = append(ics, interceptors.NoFooAllowedInterceptor[*pb.WriteRequest])
		case "metadata":
			ics = append(ics, interceptors.MetadataInterceptor)
		case "delayed":
			ics = append(ics, interceptors.DelayedInterceptor)
		default:
			log.Fatalf("Unknown interceptor: %s", name)
		}
	}
	return gorums.WithInterceptors(ics...)
}
