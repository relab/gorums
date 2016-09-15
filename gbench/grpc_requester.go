package gbench

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	rpc "github.com/relab/gorums/dev"
	"github.com/tylertreat/bench"

	"google.golang.org/grpc"
)

// RequesterFactory implements RequesterFactory by creating a Requester which
// issues requests to a register using the gRPC framework.
type GrpcRequesterFactory struct {
	Addr              string
	PayloadSize       int
	Timeout           time.Duration
	WriteRatioPercent int
}

// GetRequester returns a new Requester, called for each Benchmark connection.
func (r *GrpcRequesterFactory) GetRequester(uint64) bench.Requester {
	return &grpcRequester{
		addr: r.Addr,
		dialOpts: []grpc.DialOption{
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithTimeout(time.Second),
		},
		payloadSize: r.PayloadSize,
		timeout:     r.Timeout,
		writeRatio:  r.WriteRatioPercent,
	}
}

type grpcRequester struct {
	addr        string
	dialOpts    []grpc.DialOption
	payloadSize int
	timeout     time.Duration
	writeRatio  int

	conn   *grpc.ClientConn
	client rpc.RegisterClient
	state  *rpc.State
	ctx    context.Context
}

func (gr *grpcRequester) Setup() error {
	var err error
	gr.conn, err = grpc.Dial(gr.addr, gr.dialOpts...)
	if err != nil {
		return err
	}

	gr.ctx = context.Background()
	gr.client = rpc.NewRegisterClient(gr.conn)

	// Set initial state.
	gr.state = &rpc.State{
		Value:     strings.Repeat("x", gr.payloadSize),
		Timestamp: time.Now().UnixNano(),
	}

	wreply, err := gr.client.Write(gr.ctx, gr.state)
	if err != nil {
		return fmt.Errorf("write rpc error: %v", err)
	}
	if !wreply.New {
		return fmt.Errorf("intital write reply was not marked as new")
	}

	return nil
}

func (gr *grpcRequester) Request() error {
	var err error
	switch gr.writeRatio {
	case 0:
		_, err = gr.client.Read(gr.ctx, &rpc.ReadRequest{})
	case 100:
		gr.state.Timestamp = time.Now().UnixNano()
		_, err = gr.client.Write(gr.ctx, gr.state)
	default:
		x := rand.Intn(100)
		if x < gr.writeRatio {
			gr.state.Timestamp = time.Now().UnixNano()
			_, err = gr.client.Write(gr.ctx, gr.state)
		} else {
			_, err = gr.client.Read(gr.ctx, &rpc.ReadRequest{})
		}
	}

	return err
}

func (gr *grpcRequester) Teardown() error {
	return gr.conn.Close()
}
