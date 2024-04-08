package gorums

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

type BroadcastRouter struct {
	sendMutex         sync.Mutex
	id                uint32
	addr              string
	serverHandlers    map[string]serverHandler // handlers on other servers
	clientHandlers    map[string]clientHandler // handlers on client servers
	connections       map[string]*grpc.ClientConn
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
		dialOpts:          dialOpts,
		dialTimeout:       100 * time.Millisecond,
		connectionTimeout: 1 * time.Minute,
		doneChan:          make(chan struct{}),
		logger:            logger,
	}
}

func (r *BroadcastRouter) addAddr(id uint32, addr string) {
	r.id = id
	r.addr = addr
}

func (r *BroadcastRouter) addServerHandler(method string, handler serverHandler) {
	r.serverHandlers[method] = handler
}

func (r *BroadcastRouter) addClientHandler(method string, handler clientHandler) {
	r.clientHandlers[method] = handler
}

func (r *BroadcastRouter) send(broadcastID string, data content, req any) error {
	switch val := req.(type) {
	case *reply:
		return r.routeClientReply(broadcastID, data, val)
	case *broadcastMsg:
		return r.routeBroadcast(broadcastID, data, val)
	}
	return errors.New("wrong req type")
}

func (r *BroadcastRouter) routeBroadcast(broadcastID string, data content, msg *broadcastMsg) error {
	if handler, ok := r.serverHandlers[msg.method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see (srv *broadcastServer) registerBroadcastFunc(method string).
		handler(msg.ctx, msg.request, broadcastID, data.getOriginAddr(), data.getOriginMethod(), msg.options, r.id, r.addr)
		r.logger.Debug("broadcast took", "time", data.getProcessingTime())
		return nil
	}
	return errors.New("not found")
}

func (r *BroadcastRouter) getConnection(addr string) (*grpc.ClientConn, error) {
	if conn, ok := r.connections[addr]; ok {
		return conn, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.dialTimeout)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr, r.dialOpts...)
	if err != nil {
		return nil, err
	}
	// make sure the connection is closed
	go func() {
		time.Sleep(r.connectionTimeout)
		cc.Close()
	}()
	r.connections[addr] = cc
	return cc, err
}

func (r *BroadcastRouter) lock() {
	r.sendMutex.Lock()
}

func (r *BroadcastRouter) unlock() {
	r.sendMutex.Unlock()
}

func (r *BroadcastRouter) routeClientReply(broadcastID string, data content, resp *reply) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if handler, ok := r.clientHandlers[data.getClientHandlerName()]; ok && data.getOriginAddr() != "" {
		cc, err := r.getConnection(data.getOriginAddr())
		if err != nil {
			return err
		}
		handler(broadcastID, resp.getResponse(), cc, r.dialTimeout)
		r.logger.Debug("return took", "time", data.getProcessingTime())
		return nil
	}
	// there is a direct connection to the client, e.g. from a QuorumCall
	if data.isFromClient() {
		msg := WrapMessage(data.getMetadata(), protoreflect.ProtoMessage(resp.getResponse()), resp.getError())
		SendMessage(data.getCtx(), data.getFinished(), msg)
		r.logger.Debug("return took", "time", data.getProcessingTime())
		return nil
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	return errors.New("not routed")
}
