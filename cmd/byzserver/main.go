package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/relab/gorums/byzq"
)

type register struct {
	sync.RWMutex
	state map[string]byzq.Value
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	f := flag.Int("f", 0, "fault tolerance, supported values f=1,2,3 (this is ignored if addrs is provided)")
	key := flag.String("key", "", "public/private key file this server")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *f > 0 {
		// we are running only local since we have asked for 3f+1 servers
		done := make(chan bool)
		n := 3**f + 1
		for i := 0; i < n; i++ {
			go serve(*port+i, *key)
		}
		// wait indefinitely
		<-done
	}
	// run only one server
	serve(*port, *key)
}

func serve(port int, keyFile string) {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	if keyFile == "" {
		log.Fatalln("required server keys not provided")
	}
	creds, err := credentials.NewServerTLSFromFile(keyFile+".pem", keyFile+".key")
	if err != nil {
		log.Fatalf("failed to load credentials %v", err)
	}
	opts := []grpc.ServerOption{grpc.Creds(creds)}
	grpcServer := grpc.NewServer(opts...)
	smap := make(map[string]byzq.Value)
	byzq.RegisterRegisterServer(grpcServer, &register{state: smap})
	log.Printf("Server %s running", l.Addr())
	log.Fatal(grpcServer.Serve(l))
}

func (r *register) Read(ctx context.Context, k *byzq.Key) (*byzq.Value, error) {
	r.RLock()
	value := r.state[k.Key]
	r.RUnlock()
	return &value, nil
}

func (r *register) Write(ctx context.Context, v *byzq.Value) (*byzq.WriteResponse, error) {
	wr := &byzq.WriteResponse{Timestamp: v.C.Timestamp}
	r.Lock()
	val, found := r.state[v.C.Key]
	if !found || v.C.Timestamp > val.C.Timestamp {
		r.state[v.C.Key] = *v
	}
	r.Unlock()
	return wr, nil
}
