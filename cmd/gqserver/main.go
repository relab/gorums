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
	"google.golang.org/grpc/codes"
)

var rerr = grpc.Errorf(codes.Internal, "something very wrong happened")

type storage struct {
	sync.Mutex
	row, col uint32
	state    gridq.State

	doSleep bool
	errRate int
	err     error
}

func main() {
	var (
		port  = flag.String("port", "8080", "port to listen on")
		id    = flag.String("id", "", "id using the form 'row:col'")
		sleep = flag.Bool("sleep", false, "random sleep, [0-100) ms, before processing any request")
		erate = flag.Int("erate", 0, "reply with an error to x `percent` of requests")
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

	if *erate < 0 || *erate > 100 {
		fmt.Fprintf(os.Stderr, "error rate most be a percentage (0-100), got %d\n", *erate)
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

	if *sleep || *erate != 0 {
		rand.Seed(time.Now().Unix())
	}

	s := &storage{
		row:     row,
		col:     col,
		doSleep: *sleep,
		errRate: *erate,
		err:     rerr,
	}

	grpcServer := grpc.NewServer()
	gridq.RegisterStorageServer(grpcServer, s)
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

func (r *storage) Read(ctx context.Context, e *gridq.Empty) (*gridq.ReadResponse, error) {
	r.sleep()
	if err := r.returnErr(); err != nil {
		return nil, err
	}
	r.Lock()
	state := r.state
	r.Unlock()
	return &gridq.ReadResponse{
		Row:   r.row,
		Col:   r.col,
		State: &state,
	}, nil
}

func (r *storage) Write(ctx context.Context, s *gridq.State) (*gridq.WriteResponse, error) {
	r.sleep()
	if err := r.returnErr(); err != nil {
		return nil, err
	}
	wresp := &gridq.WriteResponse{}
	r.Lock()
	if s.Timestamp > r.state.Timestamp {
		r.state = *s
		wresp.New = true
	}
	r.Unlock()
	return wresp, nil
}

func (r *storage) returnErr() error {
	if r.errRate == 0 {
		return nil
	}
	if x := rand.Intn(100); x < r.errRate {
		return r.err
	}
	return nil
}

func (r *storage) sleep() {
	if !r.doSleep {
		return
	}
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
}
