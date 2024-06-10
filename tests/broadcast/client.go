package broadcast

import (
	"log/slog"
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

func (qs *testQSpec) BroadcastCallToQF(in *Request, replies []*Response) (*Response, bool) {
	//slog.Warn("client received reply", "resps", len(replies))
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			return resp, true
		}
	}
	return nil, false
}

func (qs *testQSpec) SearchQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) < qs.quorumSize {
		return nil, false
	}
	numCorrect := 0
	for _, resp := range replies {
		if resp.Result == 1 {
			numCorrect++
		}
	}
	if numCorrect == 1 {
		return &Response{Result: 1}, true
	}
	slog.Info("got wrong res", "replies", replies)
	return &Response{Result: 0}, true
}

func (qs *testQSpec) LongRunningTaskQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) >= qs.quorumSize {
		return nil, true
	}
	return nil, false
}

func (qs *testQSpec) GetValQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) >= qs.quorumSize {
		for _, reply := range replies {
			// all responses should be cancelled
			if reply.GetResult() != 1 {
				return &Response{Result: 0}, true
			}
		}
		return &Response{Result: 1}, true
	}
	return nil, false
}

func (qs *testQSpec) OrderQF(in *Request, replies []*Response) (*Response, bool) {
	if len(replies) >= qs.quorumSize {
		for _, resp := range replies {
			if resp.GetResult() != 0 {
				return resp, true
			}
		}
		return &Response{
			Result: 0,
		}, true
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
		//lis, err := net.Listen("tcp", "127.0.0.1:")
		lis, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, nil, err
		}
		err = mgr.AddClientServer(lis, lis.Addr())
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
