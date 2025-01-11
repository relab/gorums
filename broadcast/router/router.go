package router

import (
	"context"
	"errors"
	"github.com/relab/gorums/broadcast/dtos"
	errs "github.com/relab/gorums/broadcast/errors"
	"log/slog"
	"sync"
	"time"

	"github.com/relab/gorums/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Router is defined as an interface in order to allow mocking it in tests.
type Router interface {
	Broadcast(dto *dtos.BroadcastMsg) error
	ReplyToClient(dto *dtos.ReplyMsg) error
	Connect(addr string)
	Close() error
}

type router struct {
	mut            sync.RWMutex
	id             uint32
	addr           string
	serverHandlers map[string]ServerHandler // handlers on other servers
	clientHandlers map[string]struct{}      // specifies what handlers a client has implemented. Used only for BroadcastCalls.
	createClient   func(addr string, dialOpts []grpc.DialOption) (*dtos.Client, error)
	dialOpts       []grpc.DialOption
	dialTimeout    time.Duration
	logger         *slog.Logger
	connPool       *ConnPool
	allowList      map[string]string // whitelist of (address, pubKey) pairs the server can reply to
}

type Config struct {
	ID           uint32
	Addr         string
	Logger       *slog.Logger
	CreateClient func(addr string, dialOpts []grpc.DialOption) (*dtos.Client, error)
	DialTimeout  time.Duration
	AllowList    map[string]string
	DialOpts     []grpc.DialOption
}

func NewRouter(config *Config) *router {
	if len(config.DialOpts) <= 0 {
		config.DialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}
	return &router{
		id:             config.ID,
		addr:           config.Addr,
		serverHandlers: make(map[string]ServerHandler),
		clientHandlers: make(map[string]struct{}),
		createClient:   config.CreateClient,
		dialOpts:       config.DialOpts,
		dialTimeout:    config.DialTimeout,
		logger:         config.Logger,
		allowList:      config.AllowList,
		connPool:       newConnPool(),
	}
}
func (r *router) Connect(addr string) {
	_, _ = r.getClient(addr)
}

func (r *router) Broadcast(dto *dtos.BroadcastMsg) error {
	if handler, ok := r.serverHandlers[dto.Info.Method]; ok {
		// it runs an interceptor prior to broadcastCall, hence a different signature.
		// see: (srv *broadcastServer) registerBroadcastFunc(method string).
		handler(dto)
		return nil
	}
	err := errors.New("handler not found")
	r.log("router (broadcast): could not find handler", err, &dto.Info)
	return err
}

func (r *router) ReplyToClient(dto *dtos.ReplyMsg) error {
	// the client has initiated a broadcast call and the reply should be sent as an RPC
	if _, ok := r.clientHandlers[dto.Info.Method]; ok && dto.ClientAddr != "" {
		client, err := r.getClient(dto.ClientAddr)
		if err != nil {
			//r.log("router (reply): could not get client", err, logging.BroadcastID(dto.BroadcastID), logging.NodeAddr(dto.Addr), logging.Method(dto.Method))
			return err
		}
		err = client.SendMsg(r.dialTimeout, dto)
		r.log("router (reply): sending reply to client", err, &dto.Info)
		return err
	}
	// the server can receive a broadcast from another server before a client sends a direct message.
	// it should thus wait for a potential message from the client. otherwise, it should be removed.
	err := errors.New("not routed")
	r.log("router (reply): could not find handler", err, &dto.Info)
	return err
}

func (r *router) validAddr(addr string) bool {
	if addr == "" {
		return false
	}
	if addr == ServerOriginAddr {
		return false
	}
	if r.allowList != nil {
		_, ok := r.allowList[addr]
		return ok
	}
	return true
}

func (r *router) getClient(addr string) (*dtos.Client, error) {
	if !r.validAddr(addr) {
		return nil, errs.InvalidAddrErr{Addr: addr}
	}
	// fast path:
	// read lock because it is likely that we will send many
	// messages to the same client.
	r.mut.RLock()
	if client, ok := r.connPool.getClient(addr); ok {
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
	if client, ok := r.connPool.getClient(addr); ok {
		return client, nil
	}
	client, err := r.createClient(addr, r.dialOpts)
	if err != nil {
		return nil, err
	}
	r.connPool.addClient(addr, client)
	return client, nil
}

func (r *router) log(msg string, err error, info *dtos.Info) {
	if r.logger != nil {
		args := []slog.Attr{logging.BroadcastID(info.BroadcastID), logging.NodeAddr(info.Addr), logging.Method(info.Method), logging.Err(err), logging.Type("router")}
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}
		r.logger.LogAttrs(context.Background(), level, msg, args...)
	}
}

func (r *router) Close() error {
	return r.connPool.Close()
}

func (r *router) AddHandler(method string, handler any) {
	switch h := handler.(type) {
	case ServerHandler:
		r.serverHandlers[method] = h
	default:
		// only needs to know whether the handler exists. routing is done
		// client-side using the provided metadata in the request.
		r.clientHandlers[method] = struct{}{}
	}
}
