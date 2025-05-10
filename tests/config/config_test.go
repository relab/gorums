package config

import (
	"context"
	fmt "fmt"
	"testing"

	gorums "github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type cfgSrv struct {
	name string
}

func (srv cfgSrv) Config(_ gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		Name: srv.name,
		Num:  req.GetNum(),
	}.Build(), nil
}

// setup returns a new configuration of cfgSize and a corresponding teardown function.
// Calling setup multiple times will return a different configuration with different
// sets of nodes.
func setup(t *testing.T, mgr *Manager, cfgSize int) (cfg *Configuration, teardown func()) {
	t.Helper()
	srvs := make([]*cfgSrv, cfgSize)
	for i := range srvs {
		srvs[i] = &cfgSrv{}
	}
	addrs, closeServers := gorums.TestSetup(t, cfgSize, func(i int) gorums.ServerIface {
		srv := gorums.NewServer()
		RegisterConfigTestServer(srv, srvs[i])
		return srv
	})
	for i := range srvs {
		srvs[i].name = addrs[i]
	}
	cfg, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}
	teardown = func() {
		mgr.Close()
		closeServers()
	}
	return cfg, teardown
}

func configQF(cfg *Configuration, req *Request) (*Response, error) {
	quorum := cfg.Size()/2 + 1
	replyCount := int(0)
	replies := cfg.Config(context.Background(), req)
	for reply := range replies.IgnoreErrors() {
		replyCount++
		if replyCount < quorum {
			continue
		}
		return reply.Msg, nil
	}
	return nil, fmt.Errorf("configQF: no quorum got: %d, want (at least): %d", replyCount, quorum)
}

// TestConfig creates and combines multiple configurations and invokes the Config RPC
// method on the different configurations created below.
func TestConfig(t *testing.T) {
	callRPC := func(cfg *Configuration) {
		for i := range 5 {
			reply, err := configQF(cfg, Request_builder{Num: uint64(i)}.Build())
			if err != nil {
				t.Fatal(err)
			}
			if reply == nil {
				t.Fatal("Got nil response")
			}
		}
	}
	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	c1, teardown1 := setup(t, mgr, 4)
	defer teardown1()
	fmt.Println("--- c1 ", c1.Nodes())
	callRPC(c1)

	c2, teardown2 := setup(t, mgr, 2)
	defer teardown2()
	fmt.Println("--- c2 ", c2.Nodes())
	callRPC(c2)

	newNodeList := c1.And(c2)
	c3, err := mgr.NewConfiguration(newNodeList)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c3 ", c3.Nodes())
	callRPC(c3)

	rmNodeList := c3.Except(c1)
	c4, err := mgr.NewConfiguration(rmNodeList)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c4 ", c4.Nodes())
	callRPC(c4)
}
