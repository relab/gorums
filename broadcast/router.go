package broadcast

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Client struct {
	Addr    string
	SendMsg func(broadcastID uint64, method string, msg protoreflect.ProtoMessage, timeout time.Duration) error
	Close   func() error
}

type Router interface {
	Send(broadcastID uint64, addr, method string, req any) error
}

type BroadcastRouter struct {
	mut               sync.Mutex
	id                uint32
	addr              string
	prevMethod        uint16
	methodsConversion map[string]uint16
	serverHandlers    map[string]ServerHandler // handlers on other servers
	clientHandlers    map[string]struct{}      // specifies what handlers a client has implemented. Used only for BroadcastCalls.
	clients           map[string]*Client
	createClient      func(addr string, dialOpts []grpc.DialOption) (*Client, error)
	dialOpts          []grpc.DialOption
	dialTimeout       time.Duration
	logger            *slog.Logger
	metrics           *Metric
	state             *BroadcastState
}

func NewRouter(logger *slog.Logger, metrics *Metric, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error), dialOpts ...grpc.DialOption) *BroadcastRouter {
	if len(dialOpts) <= 0 {
		dialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}
	return &BroadcastRouter{
		serverHandlers: make(map[string]ServerHandler),
		clientHandlers: make(map[string]struct{}),
		clients:        make(map[string]*Client),
		createClient:   createClient,
		dialOpts:       dialOpts,
		dialTimeout:    3 * time.Second,
		logger:         logger,
		metrics:        metrics,
	}
}

func (r *BroadcastRouter) Send(broadcastID uint64, addr, method string, req any) error {
	switch val := req.(type) {
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, addr, method, val)
	case *reply:
		return r.routeClientReply(broadcastID, addr, method, val)
	}
	return errors.New("wrong req type")
}

func (r *BroadcastRouter) routeBroadcast(broadcastID uint64, addr, method string, msg *broadcastMsg) error {
	if handler, ok := r.serverHandlers[msg.method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		go handler(msg.ctx, msg.request, broadcastID, addr, method, msg.options, r.id, r.addr)
		return nil
	}
	return errors.New("not found")
}

func (r *BroadcastRouter) routeClientReply(broadcastID uint64, addr, method string, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if _, ok := r.clientHandlers[method]; ok && addr != "" {
		client, err := r.getClient(addr)
		if err != nil {
			return err
		}
		return client.SendMsg(broadcastID, method, resp.getResponse(), r.dialTimeout)
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}

func (r *BroadcastRouter) getClient(addr string) (*Client, error) {
	r.mut.Lock()
	defer r.mut.Unlock()
	if client, ok := r.clients[addr]; ok {
		return client, nil
	}
	client, err := r.createClient(addr, r.dialOpts)
	if err != nil {
		return nil, err
	}
	r.clients[addr] = client
	return client, nil
}

type Msg struct {
	Broadcast   bool
	BroadcastID uint64
	Msg         *broadcastMsg
	Method      string
	Reply       *reply
	//receiveChan chan error
}

type broadcastMsg struct {
	request     protoreflect.ProtoMessage
	method      string
	broadcastID uint64
	options     BroadcastOptions
	ctx         context.Context
}

func NewMsg(broadcastID uint64, req protoreflect.ProtoMessage, method string, options BroadcastOptions) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		broadcastID: broadcastID,
		options:     options,
		ctx:         context.Background(),
	}
}

type reply struct {
	Response protoreflect.ProtoMessage
	Err      error
}

func NewReply(response protoreflect.ProtoMessage, err error) *reply {
	return &reply{
		Response: response,
		Err:      err,
	}
}

func (r *reply) getResponse() protoreflect.ProtoMessage {
	return r.Response
}

func (r *reply) getError() error {
	return r.Err
}
