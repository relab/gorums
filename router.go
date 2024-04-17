package gorums

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type IBroadcastRouter interface {
	Send(broadcastID uint64, addr, method string, msg any) error
	SendOrg(broadcastID uint64, data content, msg any) error
	CreateConnection(addr string)
	AddAddr(id uint32, addr string)
	AddServerHandler(method string, handler serverHandler)
	AddClientHandler(method string, handler clientHandler)
}

type BroadcastRouter struct {
	mut               sync.Mutex
	id                uint32
	addr              string
	serverHandlers    map[string]serverHandler // handlers on other servers
	clientHandlers    map[string]clientHandler // handlers on client servers
	connections       map[string]*grpc.ClientConn
	connMutexes       map[string]*sync.Mutex
	connectionTimeout time.Duration
	dialOpts          []grpc.DialOption
	dialTimeout       time.Duration
	doneChan          chan struct{}
	logger            *slog.Logger
}

func newBroadcastRouter(logger *slog.Logger, dialOpts ...grpc.DialOption) *BroadcastRouter {
	if len(dialOpts) <= 0 {
		dialOpts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	return &BroadcastRouter{
		serverHandlers:    make(map[string]serverHandler),
		clientHandlers:    make(map[string]clientHandler),
		connections:       make(map[string]*grpc.ClientConn),
		connMutexes:       make(map[string]*sync.Mutex),
		dialOpts:          dialOpts,
		dialTimeout:       100 * time.Millisecond,
		connectionTimeout: 1 * time.Minute,
		doneChan:          make(chan struct{}),
		logger:            logger,
	}
}

func (r *BroadcastRouter) AddAddr(id uint32, addr string) {
	r.id = id
	r.addr = addr
}

func (r *BroadcastRouter) AddServerHandler(method string, handler serverHandler) {
	r.serverHandlers[method] = handler
}

func (r *BroadcastRouter) AddClientHandler(method string, handler clientHandler) {
	r.clientHandlers[method] = handler
}

func (r *BroadcastRouter) SendOrg(broadcastID uint64, data content, req any) error {
	switch val := req.(type) {
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, data.getOriginAddr(), data.getOriginMethod(), val)
	case *reply:
		return r.routeClientReply(broadcastID, data, val)
	}
	return errors.New("wrong req type")
}

func (r *BroadcastRouter) Send(broadcastID uint64, addr, method string, req any) error {
	switch val := req.(type) {
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, addr, method, val)
	case *reply:
		return r.routeClientReply2(broadcastID, addr, method, val)
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

func (r *BroadcastRouter) routeClientReply(broadcastID uint64, data content, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if handler, ok := r.clientHandlers[data.getClientHandlerName()]; ok && data.getOriginAddr() != "" {
		cc, err := r.getConnection(data.getOriginAddr())
		if err != nil {
			return err
		}
		go handler(broadcastID, resp.getResponse(), cc, r.dialTimeout)
		r.logger.Debug("return took", "time", data.getProcessingTime())
		return nil
	}
	// there is a direct connection to the client, e.g. from a QuorumCall
	if data.isFromClient() {
		msg := WrapMessage(data.getMetadata(), protoreflect.ProtoMessage(resp.getResponse()), resp.getError())
		SendMessage(data.getCtx(), data.getFinished(), msg)
		r.logger.Debug("return took", "time", data.getProcessingTime(), "data", data.client)
		return nil
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}

func (r *BroadcastRouter) routeClientReply2(broadcastID uint64, addr, method string, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if handler, ok := r.clientHandlers[method]; ok && addr != "" {
		cc, err := r.getConnection(addr)
		if err != nil {
			return err
		}
		go handler(broadcastID, resp.getResponse(), cc, r.dialTimeout)
		return nil
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}

func (r *BroadcastRouter) getConnMutex(addr string) *sync.Mutex {
	r.mut.Lock()
	defer r.mut.Unlock()
	mut, ok := r.connMutexes[addr]
	if !ok {
		mut = &sync.Mutex{}
		r.connMutexes[addr] = mut
	}
	return mut
}

func (r *BroadcastRouter) dial(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.dialTimeout)
	defer cancel()
	return grpc.DialContext(ctx, addr, r.dialOpts...)
}

func (r *BroadcastRouter) getConn(addr string) (*grpc.ClientConn, bool) {
	r.mut.Lock()
	defer r.mut.Unlock()
	cc, ok := r.connections[addr]
	return cc, ok
}

func (r *BroadcastRouter) addConn(addr string, cc *grpc.ClientConn) {
	r.mut.Lock()
	defer r.mut.Unlock()
	r.connections[addr] = cc
}

func (r *BroadcastRouter) getConnection(addr string) (*grpc.ClientConn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	addr = tcpAddr.String()
	mut := r.getConnMutex(addr)
	mut.Lock()
	defer mut.Unlock()
	//slog.Info("just sleeping a bit", "addr", addr, "id", r.addr)
	//time.Sleep(100 * time.Millisecond)
	if conn, ok := r.getConn(addr); ok {
		//slog.Info("cc cached", "addr", addr, "id", r.addr)
		return conn, nil
	}
	//slog.Info("dialing", "addr", addr, "id", r.addr)
	cc, err := r.dial(addr)
	if err != nil {
		return nil, err
	}
	// make sure the connection is closed
	go func() {
		time.Sleep(r.connectionTimeout)
		cc.Close()
	}()
	r.addConn(addr, cc)
	return cc, err
}

func (r *BroadcastRouter) CreateConnection(addr string) {
	r.getConnection(addr)
}

//func (r *BroadcastRouter) lock() {
//r.connMutex.Lock()
//}

//func (r *BroadcastRouter) unlock() {
//r.connMutex.Unlock()
//}
