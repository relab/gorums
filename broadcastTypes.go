package gorums

import (
	"strings"
	"time"

	"github.com/relab/gorums/broadcast"
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	BroadcastID string = "broadcastID"
)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastHandlerFunc func(method string, req protoreflect.ProtoMessage, broadcastID uint64, options ...broadcast.BroadcastOptions)
type BroadcastForwardHandlerFunc func(req RequestTypes, method string, broadcastID uint64, forwardAddr, originAddr string)
type BroadcastServerHandlerFunc func(method string, req RequestTypes, options ...broadcast.BroadcastOptions)
type BroadcastSendToClientHandlerFunc func(broadcastID uint64, resp protoreflect.ProtoMessage, err error)

type defaultImplementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)

type implementationFunc[T RequestTypes, V Broadcaster] func(ServerCtx, T, V)

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
}

func NewBroadcastOrchestrator(srv *Server) *BroadcastOrchestrator {
	return &BroadcastOrchestrator{
		BroadcastHandler:       srv.broadcastSrv.broadcastHandler,
		ForwardHandler:         srv.broadcastSrv.forwardHandler,
		ServerBroadcastHandler: srv.broadcastSrv.serverBroadcastHandler,
		SendToClientHandler:    srv.broadcastSrv.sendToClientHandler,
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
	return broadcast.BroadcastOptions{}
}

type Broadcaster interface{}

type BroadcastMetadata struct {
	BroadcastID       uint64
	IsBroadcastClient bool // type of sender, could be: Client or Server
	//SenderID     uint32 // nodeID of last hop
	SenderAddr   string // address of last hop
	OriginAddr   string // address of the origin
	OriginMethod string // the first method called by the origin
	Method       string // the current method
	Digest       []byte // digest of original message sent by client
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
	return BroadcastMetadata{
		BroadcastID:       md.BroadcastMsg.BroadcastID,
		IsBroadcastClient: md.BroadcastMsg.IsBroadcastClient,
		//SenderID:     md.BroadcastMsg.SenderID,
		SenderAddr:   md.BroadcastMsg.SenderAddr,
		OriginAddr:   md.BroadcastMsg.OriginAddr,
		OriginMethod: md.BroadcastMsg.OriginMethod,
		Method:       m,
	}
}
