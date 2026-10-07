package config

import (
	"testing"
	"time"

	gorums "github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

type cfgSrv struct{}

func (cfgSrv) Read(_ gorums.ServerContext, req *Request) (resp *Response, err error) {
	return Response_builder{
		Num: req.GetNum(),
	}.Build(), nil
}

func serverFn(_ int) gorumstest.ServerIface {
	srv := gorums.NewServer()
	RegisterConfigTestServer(srv, &cfgSrv{})
	return srv
}

// TestConfig creates and combines multiple configurations and invokes the Read RPC
// method on the different configurations created below.
func TestConfig(t *testing.T) {
	callRPC := func(config Config) {
		cfgCtx := config.Context(gorumstest.Context(t, 5*time.Second))
		for i := range 5 {
			resp, err := Read(cfgCtx,
				Request_builder{Num: uint64(i)}.Build()).Majority()
			if err != nil {
				t.Fatal(err)
			}
			if resp == nil {
				t.Fatal("Got nil response")
			}
		}
	}

	c1 := gorumstest.Config(t, 6, serverFn)
	callRPC(c1)

	// Create c2 by removing 2 nodes from c1.
	c2 := c1.Remove(1, 2)
	callRPC(c2)

	// Create c3 = c1 ∪ c2
	c3 := c1.Union(c2)
	callRPC(c3)

	// Create c4 = c3 \ c2
	c4 := c3.Difference(c2)
	callRPC(c4)
}
