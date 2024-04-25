package broadcast

import (
	net "net"

	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testQSpec struct {
	quorumSize    int
	broadcastSize int
}

func newQSpec(qSize, broadcastSize int) QuorumSpec {
	return &testQSpec{
		quorumSize:    qSize,
		broadcastSize: broadcastSize,
	}
}

func (qs *testQSpec) QuorumCallQF(in *Request, replies map[uint32]*Response) (*Response, bool) {
	//slog.Warn("client received reply")
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func (qs *testQSpec) QuorumCallWithBroadcastQF(in *Request, replies map[uint32]*Response) (*Response, bool) {
	//slog.Warn("client received reply")
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func (qs *testQSpec) QuorumCallWithMulticastQF(in *Request, replies map[uint32]*Response) (*Response, bool) {
	//slog.Warn("client received reply")
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func (qs *testQSpec) BroadcastCallQF(in *Request, replies []*Response) (*Response, bool) {
	//slog.Warn("client received reply", "val", in.Value, "resps", len(replies))
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func (qs *testQSpec) BroadcastCallForwardQF(in *Request, replies []*Response) (*Response, bool) {
	//slog.Warn("client received reply", "resps", len(replies))
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func newClient(srvAddrs []string, listenAddr string, qsize ...int) (*Configuration, func(), error) {
	quorumSize := len(srvAddrs)
	if len(qsize) > 0 {
		quorumSize = qsize[0]
	}
	mgr := NewManager(
		gorums.WithPublicKey("client"),
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	if listenAddr != "" {
		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, nil, err
		}
		err = mgr.AddClientServer2(lis)
		if err != nil {
			return nil, nil, err
		}
	}
	config, err := mgr.NewConfiguration(
		gorums.WithNodeList(srvAddrs),
		newQSpec(quorumSize, quorumSize),
	)
	if err != nil {
		return nil, nil, err
	}
	return config, func() {
		mgr.Close()
	}, nil
}
