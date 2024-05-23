package gorums

import (
	"context"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastServer struct {
	propertiesMutex   sync.Mutex
	viewMutex         sync.RWMutex
	id                uint32
	addr              string
	machineID         uint64
	view              RawConfiguration
	createBroadcaster func(m BroadcastMetadata, o *BroadcastOrchestrator, b EnqueueBroadcast) Broadcaster
	orchestrator      *BroadcastOrchestrator
	manager           broadcast.Manager
	logger            *slog.Logger
}

func (srv *Server) GetStats() broadcast.Metrics {
	return srv.broadcastSrv.manager.GetStats()
}

func newBroadcastServer(logger *slog.Logger, order map[string]int, machineID uint64) *broadcastServer {
	srv := &broadcastServer{
		logger:    logger,
		machineID: machineID,
	}
	srv.manager = broadcast.NewBroadcastManager(logger, createClient, srv.canceler, order)
	return srv
}

func (srv *broadcastServer) stop() {
	srv.manager.Close()
}

type Snowflake interface {
	NewBroadcastID() uint64
}

func (srv *broadcastServer) addAddr(addr string) {
	srv.propertiesMutex.Lock()
	defer srv.propertiesMutex.Unlock()
	//if srv.addr != "" {
	//return
	//}
	//t := strings.Split(addr, ":")
	//if len(t) < 2 || t[0] == "" || t[0] == "0.0.0.0" {
	//panic(fmt.Sprintf("addr cannot be 0.0.0.0 or empty. got: %s", addr))
	//}
	srv.addr = addr
	h := fnv.New32a()
	_, _ = h.Write([]byte(srv.addr))
	srv.id = h.Sum32()
	srv.manager.AddAddr(srv.id, srv.addr, srv.machineID)
}

const (
	BroadcastID string = "broadcastID"
)

type BroadcastHandlerFunc func(method string, req protoreflect.ProtoMessage, broadcastID uint64, enqueueBroadcast EnqueueBroadcast, options ...broadcast.BroadcastOptions) error
type BroadcastForwardHandlerFunc func(req protoreflect.ProtoMessage, method string, broadcastID uint64, forwardAddr, originAddr string)
type BroadcastServerHandlerFunc func(method string, req protoreflect.ProtoMessage, options ...broadcast.BroadcastOptions)
type BroadcastSendToClientHandlerFunc func(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueBroadcast EnqueueBroadcast) error
type CancelHandlerFunc func(broadcastID uint64, srvAddrs []string)
type DoneHandlerFunc func(broadcastID uint64)
type EnqueueBroadcast func(broadcast.Msg) error

type defaultImplementationFunc[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage] func(ServerCtx, T) (V, error)
type clientImplementationFunc[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage] func(context.Context, T, uint64) (V, error)

type implementationFunc[T protoreflect.ProtoMessage, V Broadcaster] func(ServerCtx, T, V)

func CancelFunc(ServerCtx, protoreflect.ProtoMessage, Broadcaster) {}

const Cancellation string = "cancel"

// The BroadcastOrchestrator is used as a container for all
// broadcast handlers. The BroadcastHandler takes in a method
// and schedules it for broadcasting. SendToClientHandler works
// similarly but it sends the message to the calling client.
//
// It is necessary to use an orchestrator to hide certain
// implementation details, such as internal methods on the
// broadcast struct. The BroadcastOrchestrator will thus
// be an unimported field in the broadcast struct in the
// generated code.
type BroadcastOrchestrator struct {
	BroadcastHandler       BroadcastHandlerFunc
	ForwardHandler         BroadcastForwardHandlerFunc
	SendToClientHandler    BroadcastSendToClientHandlerFunc
	ServerBroadcastHandler BroadcastServerHandlerFunc
	CancelHandler          CancelHandlerFunc
	DoneHandler            DoneHandlerFunc
}

func NewBroadcastOrchestrator(srv *Server) *BroadcastOrchestrator {
	return &BroadcastOrchestrator{
		BroadcastHandler:       srv.broadcastSrv.broadcastHandler,
		ForwardHandler:         srv.broadcastSrv.forwardHandler,
		ServerBroadcastHandler: srv.broadcastSrv.serverBroadcastHandler,
		SendToClientHandler:    srv.broadcastSrv.sendToClientHandler,
		CancelHandler:          srv.broadcastSrv.cancelHandler,
		DoneHandler:            srv.broadcastSrv.doneHandler,
	}
}

type BroadcastOption func(*broadcast.BroadcastOptions)

func WithSubset(srvAddrs ...string) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.ServerAddresses = srvAddrs
	}
}

func WithGossip(percentage float32, ttl int) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.GossipPercentage = percentage
		b.TTL = ttl
	}
}

func WithTTL(ttl int) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.TTL = ttl
	}
}

func WithDeadline(deadline time.Time) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.Deadline = deadline
	}
}

func WithoutSelf() BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.SkipSelf = true
	}
}

func WithoutUniquenessChecks() BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.OmitUniquenessChecks = true
	}
}

func WithRelationToRequest(broadcastID uint64) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.RelatedToReq = broadcastID
	}
}

func NewBroadcastOptions() broadcast.BroadcastOptions {
	return broadcast.BroadcastOptions{
		ServerAddresses: make([]string, 0), // to prevent nil errors
	}
}

type Broadcaster interface{}

type BroadcastMetadata struct {
	BroadcastID       uint64
	IsBroadcastClient bool      // type of sender, could be: Client or Server
	SenderAddr        string    // address of last hop
	OriginAddr        string    // address of the origin
	OriginMethod      string    // the first method called by the origin
	Method            string    // the current method
	Digest            []byte    // digest of original message sent by client
	Timestamp         time.Time // timestamp in seconds when the broadcast request was issued by the client/server
	ShardID           uint16    // ID of the shard handling the broadcast request
	MachineID         uint16    // ID of the client/server that issued the broadcast request
	SequenceNo        uint32    // sequence number of the broadcast request from that particular client/server. Will roll over when reaching max.
}

func newBroadcastMetadata(md *ordering.Metadata) BroadcastMetadata {
	if md == nil {
		return BroadcastMetadata{}
	}
	tmp := strings.Split(md.Method, ".")
	m := ""
	if len(tmp) >= 1 {
		m = tmp[len(tmp)-1]
	}
	timestamp, shardID, machineID, sequenceNo := broadcast.DecodeBroadcastID(md.BroadcastMsg.BroadcastID)
	return BroadcastMetadata{
		BroadcastID:       md.BroadcastMsg.BroadcastID,
		IsBroadcastClient: md.BroadcastMsg.IsBroadcastClient,
		SenderAddr:        md.BroadcastMsg.SenderAddr,
		OriginAddr:        md.BroadcastMsg.OriginAddr,
		OriginMethod:      md.BroadcastMsg.OriginMethod,
		Method:            m,
		Timestamp:         broadcast.Epoch().Add(time.Duration(timestamp) * time.Second),
		ShardID:           shardID,
		MachineID:         machineID,
		SequenceNo:        sequenceNo,
	}
}
