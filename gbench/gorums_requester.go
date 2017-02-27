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

// GorumsRequesterFactory implements RequesterFactory by creating a Requester which
// issues requests to a register using the Gorums framework.
type GorumsRequesterFactory struct {
	Addrs             []string
	ReadQuorum        int
	WriteQuorum       int
	PayloadSize       int
	QCTimeout         time.Duration
	WriteRatioPercent int
}

// GetRequester returns a new Requester, called for each Benchmark connection.
func (r *GorumsRequesterFactory) GetRequester(uint64) bench.Requester {
	return &gorumsRequester{
		addrs:       r.Addrs,
		readq:       r.ReadQuorum,
		writeq:      r.WriteQuorum,
		payloadSize: r.PayloadSize,
		timeout:     r.QCTimeout,
		writeRatio:  r.WriteRatioPercent,
		dialOpts: []grpc.DialOption{
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithTimeout(time.Second),
		},
	}
}

type gorumsRequester struct {
	addrs       []string
	readq       int
	writeq      int
	payloadSize int
	timeout     time.Duration
	writeRatio  int

	dialOpts []grpc.DialOption

	mgr    *rpc.Manager
	config *rpc.Configuration
	state  *rpc.State
}

func (gr *gorumsRequester) Setup() error {
	var err error
	gr.mgr, err = rpc.NewManager(
		gr.addrs,
		rpc.WithGrpcDialOptions(gr.dialOpts...),
	)
	if err != nil {
		return err
	}

	ids := gr.mgr.NodeIDs()
	qspec := newRegisterQSpec(gr.readq, gr.writeq)
	gr.config, err = gr.mgr.NewConfiguration(ids, qspec)
	if err != nil {
		return err
	}

	// Set initial state.
	gr.state = &rpc.State{
		Value:     strings.Repeat("x", gr.payloadSize),
		Timestamp: time.Now().UnixNano(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), gr.timeout)
	defer cancel()
	wreply, err := gr.config.Write(ctx, gr.state)
	if err != nil {
		return fmt.Errorf("write rpc error: %v", err)
	}
	if !wreply.New {
		return fmt.Errorf("intital write reply was not marked as new")
	}

	return nil
}

func (gr *gorumsRequester) Request() error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), gr.timeout)
	defer cancel()
	switch gr.writeRatio {
	case 0:
		_, err = gr.config.Read(ctx, &rpc.ReadRequest{})
	case 100:
		gr.state.Timestamp = time.Now().UnixNano()
		_, err = gr.config.Write(ctx, gr.state)
	default:
		x := rand.Intn(100)
		if x < gr.writeRatio {
			gr.state.Timestamp = time.Now().UnixNano()
			_, err = gr.config.Write(ctx, gr.state)
		} else {
			_, err = gr.config.Read(ctx, &rpc.ReadRequest{})
		}
	}

	return err
}

func (gr *gorumsRequester) Teardown() error {
	gr.mgr.Close()
	gr.mgr = nil
	gr.config = nil
	return nil
}

type registerQSpec struct {
	rq, wq int
}

func newRegisterQSpec(rq, wq int) rpc.QuorumSpec {
	return &registerQSpec{
		rq: rq,
		wq: wq,
	}
}

func (rqs *registerQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *registerQSpec) ReadFutureQF(replies []*rpc.State) (*rpc.State, bool) {
	return rqs.ReadQF(replies)
}

func (rqs *registerQSpec) ReadCustomReturnQF(replies []*rpc.State) (*rpc.State, bool) {
	return rqs.ReadQF(replies)
}

func (rqs *registerQSpec) ReadCorrectableQF(replies []*rpc.State) (*rpc.State, int, bool) {
	panic("not implemented")
}

func (rqs *registerQSpec) ReadPrelimQF(replies []*rpc.State) (*rpc.State, int, bool) {
	panic("not implemented")
}

func (rqs *registerQSpec) WriteQF(req *rpc.State, replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *registerQSpec) WriteFutureQF(req *rpc.State, replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	return rqs.WriteQF(req, replies)
}

func (rqs *registerQSpec) WritePerNodeQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}
