package broadcast

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/logging"
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
	Send(broadcastID uint64, addr, method string, req msg) error
	Connect(addr string)
}

type BroadcastRouter struct {
	mut               sync.RWMutex
	id                uint32
	addr              string
	prevMethod        uint16
	methodsConversion map[string]uint16
	serverHandlers    map[string]ServerHandler // handlers on other servers
	clientHandlers    map[string]struct{}      // specifies what handlers a client has implemented. Used only for BroadcastCalls.
	createClient      func(addr string, dialOpts []grpc.DialOption) (*Client, error)
	canceler          func(broadcastID uint64, srvAddrs []string)
	dialOpts          []grpc.DialOption
	dialTimeout       time.Duration
	logger            *slog.Logger
	state             *BroadcastState
}

func NewRouter(logger *slog.Logger, createClient func(addr string, dialOpts []grpc.DialOption) (*Client, error), canceler func(broadcastID uint64, srvAddrs []string), dialOpts ...grpc.DialOption) *BroadcastRouter {
	if len(dialOpts) <= 0 {
		dialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}
	return &BroadcastRouter{
		serverHandlers: make(map[string]ServerHandler),
		clientHandlers: make(map[string]struct{}),
		createClient:   createClient,
		canceler:       canceler,
		dialOpts:       dialOpts,
		dialTimeout:    3 * time.Second,
		logger:         logger,
	}
}

func (r *BroadcastRouter) registerState(state *BroadcastState) {
	r.state = state
}

type msg interface{}

func (r *BroadcastRouter) Send(broadcastID uint64, addr, method string, req msg) error {
	if r.addr == "" {
		panic("The listen addr on the broadcast server cannot be empty. Use the WithListenAddr() option when creating the server.")
	}
	switch val := req.(type) {
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, addr, method, val)
	case *reply:
		return r.routeClientReply(broadcastID, addr, method, val)
	case *cancellation:
		r.canceler(broadcastID, val.srvAddrs)
		return nil
	}
	err := errors.New("wrong req type")
	r.log("router: malformed msg", err, logging.BroadcastID(broadcastID))
	return err
}

func (r *BroadcastRouter) Connect(addr string) {
	r.getClient(addr)
}

func (r *BroadcastRouter) routeBroadcast(broadcastID uint64, addr, method string, msg *broadcastMsg) error {
	if handler, ok := r.serverHandlers[msg.method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		handler(msg.ctx, msg.request, broadcastID, addr, method, msg.options, r.id, r.addr)
		return nil
	}
	err := errors.New("handler not found")
	r.log("router (broadcast): could not find handler", err, logging.BroadcastID(broadcastID), logging.NodeAddr(addr), logging.Method(method))
	return err
}

func (r *BroadcastRouter) routeClientReply(broadcastID uint64, addr, method string, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if _, ok := r.clientHandlers[method]; ok && addr != "" {
		client, err := r.getClient(addr)
		if err != nil {
			r.log("router (reply): could not get client", err, logging.BroadcastID(broadcastID), logging.NodeAddr(addr), logging.Method(method))
			return err
		}
		err = client.SendMsg(broadcastID, method, resp.getResponse(), r.dialTimeout)
		r.log("router (reply): sending reply to client", err, logging.BroadcastID(broadcastID), logging.NodeAddr(addr), logging.Method(method))
		return err
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	err := errors.New("not routed")
	r.log("router (reply): could not find handler", err, logging.BroadcastID(broadcastID), logging.NodeAddr(addr), logging.Method(method))
	return err
}

func (r *BroadcastRouter) getClient(addr string) (*Client, error) {
	if addr == "" || addr == ServerOriginAddr {
		return nil, InvalidAddrErr{addr: addr}
	}
	// fast path:
	// read lock because it is likely that we will send many
	// messages to the same client.
	r.mut.RLock()
	if client, ok := r.state.getClient(addr); ok {
		r.mut.RUnlock()
		return client, nil
	}
	r.mut.RUnlock()
	// slow path:
	// we need a write lock when adding a new client. This only process
	// one at a time and is thus necessary to check if the client has
	// already been added again. Otherwise, we can end up creating multiple
	// clients.
	r.mut.Lock()
	defer r.mut.Unlock()
	if client, ok := r.state.getClient(addr); ok {
		return client, nil
	}
	client, err := r.createClient(addr, r.dialOpts)
	if err != nil {
		return nil, err
	}
	r.state.addClient(addr, client)
	return client, nil
}

func (r *BroadcastRouter) log(msg string, err error, args ...slog.Attr) {
	if r.logger != nil {
		args = append(args, logging.Err(err), logging.Type("router"))
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}
		r.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}

type msgType int

const (
	BroadcastMsg msgType = iota
	ReplyMsg
	CancellationMsg
)

func (m msgType) String() string {
	switch m {
	case BroadcastMsg:
		return "BroadcastMsg"
	case ReplyMsg:
		return "ReplyMsg"
	case CancellationMsg:
		return "CancellationMsg"
	}
	return "unkown"
}

type Msg struct {
	MsgType      msgType
	BroadcastID  uint64
	Msg          *broadcastMsg
	Method       string
	Reply        *reply
	Cancellation *cancellation
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

func (r *reply) getResponse() protoreflect.ProtoMessage {
	return r.Response
}

func (r *reply) getError() error {
	return r.Err
}

type cancellation struct {
	srvAddrs []string
	end      bool // end is used to stop the request.
}
