package gorums

import (
	"context"
	"crypto/elliptic"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums/authentication"
	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/logging"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// exposing the log entry struct used for structured logging to the user
type LogEntry logging.LogEntry

// exposing the ellipticCurve struct for the user
func NewAuth(curve elliptic.Curve) *authentication.EllipticCurve {
	return authentication.New(curve)
}

type broadcastServer struct {
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

func newBroadcastServer(serverOpts *serverOptions) *broadcastServer {
	h := fnv.New32a()
	_, _ = h.Write([]byte(serverOpts.listenAddr))
	id := h.Sum32()
	srv := &broadcastServer{
		id:        id,
		addr:      serverOpts.listenAddr,
		logger:    serverOpts.logger,
		machineID: serverOpts.machineID,
	}
	srv.manager = broadcast.NewBroadcastManager(serverOpts.logger, createClient, srv.canceler, serverOpts.executionOrder, serverOpts.clientDialTimeout, serverOpts.reqTTL, serverOpts.shardBuffer, serverOpts.sendBuffer, serverOpts.allowList, serverOpts.grpcDialOpts...)
	srv.manager.AddAddr(srv.id, serverOpts.listenAddr, srv.machineID)
	return srv
}

func (srv *broadcastServer) stop() {
	srv.manager.Close()
}

type Snowflake interface {
	NewBroadcastID() uint64
}

const (
	BroadcastID string = "broadcastID"
)

type (
	BroadcastHandlerFunc             func(method string, req protoreflect.ProtoMessage, broadcastID uint64, enqueueBroadcast EnqueueBroadcast, options ...broadcast.BroadcastOptions) error
	BroadcastForwardHandlerFunc      func(req protoreflect.ProtoMessage, method string, broadcastID uint64, forwardAddr, originAddr string)
	BroadcastServerHandlerFunc       func(method string, req protoreflect.ProtoMessage, options ...broadcast.BroadcastOptions)
	BroadcastSendToClientHandlerFunc func(broadcastID uint64, resp protoreflect.ProtoMessage, err error, enqueueBroadcast EnqueueBroadcast) error
	CancelHandlerFunc                func(broadcastID uint64, srvAddrs []string, enqueueBroadcast EnqueueBroadcast) error
	DoneHandlerFunc                  func(broadcastID uint64, enqueueBroadcast EnqueueBroadcast)
	EnqueueBroadcast                 func(*broadcast.Msg) error
)

type (
	defaultImplementationFunc[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage] func(ServerCtx, T) (V, error)
	clientImplementationFunc[T protoreflect.ProtoMessage, V protoreflect.ProtoMessage]  func(context.Context, T, uint64) (V, error)
)

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

// WithSubset enables broadcasting to a subset of the servers in the view.
// It has the same function as broadcast.To().
func WithSubset(srvAddrs ...string) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.ServerAddresses = srvAddrs
	}
}

// WithoutSelf prevents the server from broadcasting to itself.
func WithoutSelf() BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.SkipSelf = true
	}
}

// ProgressTo allows the server to accept messages to the given method.
// Should only be used if the ServerOption WithOrder() is used.
func ProgressTo(method string) BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.ProgressTo = method
	}
}

// AllowDuplication allows the server to broadcast more than once
// to the same RPC method for a particular broadcast request.
func AllowDuplication() BroadcastOption {
	return func(b *broadcast.BroadcastOptions) {
		b.AllowDuplication = true
	}
}

// WithRelationToRequest allows for broadcasting outside a
// server handler related to a specific broadcastID.
// It is not recommended to use this method. Use the broadcast
// struct provided with a broadcast request instead.
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
	Timestamp         time.Time // timestamp in seconds when the broadcast request was issued by the client/server
	ShardID           uint16    // ID of the shard handling the broadcast request
	MachineID         uint16    // ID of the client/server that issued the broadcast request
	SequenceNo        uint32    // sequence number of the broadcast request from that particular client/server. Will roll over when reaching max.
	OriginDigest      []byte
	OriginSignature   []byte
	OriginPubKey      string
}

func newBroadcastMetadata(md *ordering.Metadata) BroadcastMetadata {
	if md == nil {
		return BroadcastMetadata{}
	}
	tmp := strings.Split(md.GetMethod(), ".")
	m := ""
	if len(tmp) >= 1 {
		m = tmp[len(tmp)-1]
	}
	timestamp, shardID, machineID, sequenceNo := broadcast.DecodeBroadcastID(md.GetBroadcastMsg().GetBroadcastID())
	return BroadcastMetadata{
		BroadcastID:       md.GetBroadcastMsg().GetBroadcastID(),
		IsBroadcastClient: md.GetBroadcastMsg().GetIsBroadcastClient(),
		SenderAddr:        md.GetBroadcastMsg().GetSenderAddr(),
		OriginAddr:        md.GetBroadcastMsg().GetOriginAddr(),
		OriginMethod:      md.GetBroadcastMsg().GetOriginMethod(),
		OriginDigest:      md.GetBroadcastMsg().GetOriginDigest(),
		OriginSignature:   md.GetBroadcastMsg().GetOriginSignature(),
		OriginPubKey:      md.GetBroadcastMsg().GetOriginPubKey(),
		Method:            m,
		Timestamp:         broadcast.Epoch().Add(time.Duration(timestamp) * time.Second),
		ShardID:           shardID,
		MachineID:         machineID,
		SequenceNo:        sequenceNo,
	}
}

func (md BroadcastMetadata) Verify(msg protoreflect.ProtoMessage) (bool, error) {
	return authentication.Verify(md.OriginPubKey, md.OriginSignature, md.OriginDigest, msg)
}
