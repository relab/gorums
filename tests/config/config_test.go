package config

import (
	"context"
	fmt "fmt"
	"testing"
	"time"

	gorums "github.com/relab/gorums"
	"google.golang.org/grpc"
)

type (
	cfgSrv struct {
		name string
	}
	cfgQSpec struct {
		quorum int
	}
)

func (srv cfgSrv) Config(ctx context.Context, req *Request, out func(*Response, error)) {
	out(&Response{
		Name: srv.name,
		Num:  req.GetNum(),
	}, nil)
}

func newQSpec(cfgSize int) *cfgQSpec {
	return &cfgQSpec{quorum: cfgSize/2 + 1}
}

func (q cfgQSpec) ConfigQF(_ *Request, replies map[uint32]*Response) (*Response, bool) {
	if len(replies) < q.quorum {
		return nil, false
	}
	var reply *Response
	for _, r := range replies {
		reply = r
		break
	}
	return reply, true
}

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
	cfg, err := mgr.NewConfiguration(newQSpec(cfgSize), gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}
	teardown = func() {
		mgr.Close()
		closeServers()
	}
	return cfg, teardown
}

func TestConfig(t *testing.T) {
	f := func(cfg *Configuration) {
		for i := 0; i < 5; i++ {
			resp, err := cfg.Config(context.Background(), &Request{Num: uint64(i)})
			if err != nil {
				t.Fatal(err)
			}
			if resp == nil {
				t.Fatal("Got nil response")
			}
		}
	}
	mgr := NewManager(
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
	)
	c1, teardown1 := setup(t, mgr, 4)
	defer teardown1()
	fmt.Println("--- c1 ", c1.Nodes())
	f(c1)

	c2, teardown2 := setup(t, mgr, 2)
	defer teardown2()
	fmt.Println("--- c2 ", c2.Nodes())
	f(c2)

	newNodeList := c1.With(c2)
	c3, err := mgr.NewConfiguration(
		newQSpec(c1.Size()+c2.Size()),
		newNodeList,
	)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c3 ", c3.Nodes())
	f(c3)

	rmNodeList := c3.Without(c1)
	c4, err := mgr.NewConfiguration(
		newQSpec(c2.Size()),
		rmNodeList,
	)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("--- c4 ", c4.Nodes())
	f(c4)
}
