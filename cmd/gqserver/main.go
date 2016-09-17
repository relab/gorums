package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums/gridq"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type register struct {
	sync.RWMutex
	row, col uint32
	state    gridq.State
	sleep    bool
}

func main() {
	var (
		port  = flag.String("port", "8080", "port to listen on")
		id    = flag.String("id", "", "id using the form 'row:col'")
		sleep = flag.Bool("sleep", true, "random sleep (0-30ms) before processing any request")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *id == "" {
		fmt.Fprintf(os.Stderr, "no id given\n")
		flag.Usage()
		os.Exit(2)
	}

	row, col, err := parseRowCol(*id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing id: %v\n", err)
		flag.Usage()
		os.Exit(2)
	}

	l, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		fmt.Println("error listening:", err)
		os.Exit(2)
	}

	rand.Seed(time.Now().Unix())

	register := &register{
		row:   row,
		col:   col,
		sleep: *sleep,
	}

	grpcServer := grpc.NewServer()
	gridq.RegisterRegisterServer(grpcServer, register)
	fmt.Println(grpcServer.Serve(l))
	os.Exit(2)
}

func parseRowCol(id string) (uint32, uint32, error) {
	splitted := strings.Split(id, ":")
	if len(splitted) != 2 {
		return 0, 0, errors.New("id should have form 'row:col'")
	}
	row, err := strconv.Atoi(splitted[0])
	if err != nil {
		return 0, 0, fmt.Errorf("error parsing row from: %q", splitted[0])
	}
	col, err := strconv.Atoi(splitted[1])
	if err != nil {
		return 0, 0, fmt.Errorf("error parsing col from: %q", splitted[1])
	}
	return uint32(row), uint32(col), nil
}

func (r *register) Read(ctx context.Context, e *gridq.Empty) (*gridq.ReadResponse, error) {
	r.randSleep()
	r.RLock()
	state := r.state
	r.RUnlock()
	return &gridq.ReadResponse{
		Row:   r.row,
		Col:   r.col,
		State: &state,
	}, nil
}

func (r *register) Write(ctx context.Context, s *gridq.State) (*gridq.WriteResponse, error) {
	r.randSleep()
	wresp := &gridq.WriteResponse{}
	r.Lock()
	if s.Timestamp > r.state.Timestamp {
		r.state = *s
		wresp.Written = true
	}
	r.Unlock()
	return wresp, nil
}

func (r *register) randSleep() {
	if !r.sleep {
		return
	}
	dur := time.Duration(rand.Intn(30)) * time.Millisecond
	time.Sleep(dur)
}
