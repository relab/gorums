package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/relab/gorums"
	"github.com/relab/gorums/examples/interceptors"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	server := flag.String("server", "", "Start as a server on given address.")
	remotes := flag.String("connect", "", "Comma-separated list of servers to connect to.")
	ic := flag.String("interceptors", "", "Comma-separated list of interceptors to enable (logging, nofoo, metadata, delayed).")
	flag.Parse()

	if *server != "" {
		runServer(*server)
		return
	}

	srvOpts := parseInterceptors(*ic)

	addrs := strings.Split(*remotes, ",")
	// start local servers if no remote servers were specified
	if len(addrs) == 1 && addrs[0] == "" {
		systems, stop, err := gorums.NewLocalSystems(4, srvOpts...)
		if err != nil {
			log.Fatalf("Failed to create local systems: %v", err)
		}
		defer stop()

		addrs = make([]string, len(systems))
		for i, sys := range systems {
			addrs[i] = sys.Addr()
		}
		dialOpts := gorums.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials()))
		for i, sys := range systems {
			storage := newStorageServer(os.Stderr, fmt.Sprintf("node %d", i))
			// Exclude self from peer config to avoid self-connection issues in nested calls.
			peerAddrs := make([]string, 0, len(addrs)-1)
			for _, addr := range addrs {
				if addr != sys.Addr() {
					peerAddrs = append(peerAddrs, addr)
				}
			}
			cfg, err := sys.NewOutboundConfig(gorums.WithNodeList(peerAddrs), dialOpts)
			if err != nil {
				log.Fatalf("Failed to create server peer configuration: %v", err)
			}
			storage.peerCfg = cfg
			sys.RegisterService(nil, func(srv *gorums.Server) {
				pb.RegisterStorageServer(srv, storage)
			})
			go func() {
				if err := sys.Serve(); err != nil {
					log.Printf("Server error: %v", err)
				}
			}()
		}
	}

	if runClient(addrs) != nil {
		os.Exit(1)
	}
}

// parseInterceptors converts a comma-separated interceptor list into server options.
func parseInterceptors(ic string) []gorums.ServerOption {
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
	return []gorums.ServerOption{gorums.WithInterceptors(ics...)}
}
