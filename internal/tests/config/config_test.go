package config

import (
	"context"
	"fmt"
	"testing"

	gorums "github.com/relab/gorums"
)

type cfgSrv struct{}

func (srv cfgSrv) Config(ctx gorums.ServerCtx, req *Request) (resp *Response, err error) {
	return Response_builder{
		Num: req.GetNum(),
	}.Build(), nil
}

func serverFn(i int) gorums.ServerIface {
	srv := gorums.NewServer()
	RegisterConfigTestServer(srv, &cfgSrv{})
	return srv
}

// TestConfig creates and combines multiple configurations and invokes the Config RPC
// method on the different configurations created below.
func TestConfig(t *testing.T) {
	callRPC := func(cfg gorums.Configuration) {
		cfgCtx := gorums.WithConfigContext(context.Background(), cfg)
		for i := range 5 {
			// Use the new terminal method API - wait for a majority
			resp, err := Config(cfgCtx,
				Request_builder{Num: uint64(i)}.Build()).Majority()
			if err != nil {
				t.Fatal(err)
			}
			if resp == nil {
				t.Fatal("Got nil response")
			}
		}
	}

	c1 := gorums.SetupConfiguration(t, 4, serverFn)
	fmt.Println("--- c1 ", c1.Nodes())
	callRPC(c1)

	// Create a new configuration c2 with 2 new nodes not in c1, using the same manager as c1.
	c2 := gorums.SetupConfiguration(t, 2, serverFn, gorums.WithManager(t, c1.Manager()))
	fmt.Println("--- c2 ", c2.Nodes())
	callRPC(c2)

	// Create c3 = c1 ∪ c2, using the same manager as c1 (and c2).
	newNodeList := c1.And(c2)
	c3, err := gorums.NewConfiguration(c1.Manager(), newNodeList)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c3 ", c3.Nodes())
	callRPC(c3)

	// Create c4 = c3 \ c1, using the same manager as c1 (and c2, c3).
	rmNodeList := c3.Except(c1)
	c4, err := gorums.NewConfiguration(c1.Manager(), rmNodeList)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c4 ", c4.Nodes())
	callRPC(c4)
}
