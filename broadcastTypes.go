package gorums

import (
	"context"
	"strings"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	BroadcastClient string = "client"
	BroadcastServer string = "server"
	BroadcastID     string = "broadcastID"
)

type serverHandler func(ctx context.Context, in RequestTypes, broadcastID, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string)
type clientHandler func(broadcastID string, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastHandlerFunc func(method string, req RequestTypes, broadcastID string, data ...BroadcastOptions)
type BroadcastForwardHandlerFunc func(req RequestTypes, method, broadcastID, forwardAddr, originAddr string)
type BroadcastSendToClientHandlerFunc func(broadcastID string, resp ResponseTypes, err error)

type defaultImplementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)

type implementationFunc[T RequestTypes, V Broadcaster] func(ServerCtx, T, V)

type reply struct {
	response    ResponseTypes
	err         error
	broadcastID string
	timestamp   time.Time
}

func newReply(response ResponseTypes, err error, broadcastID string) *reply {
	return &reply{
		response:    response,
		err:         err,
		broadcastID: broadcastID,
		timestamp:   time.Now(),
	}
}

func (r *reply) getResponse() ResponseTypes {
	return r.response
}

func (r *reply) getError() error {
	return r.err
}

func (r *reply) getBroadcastID() string {
	return r.broadcastID
}

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
	BroadcastHandler    BroadcastHandlerFunc
	ForwardHandler      BroadcastForwardHandlerFunc
	SendToClientHandler BroadcastSendToClientHandlerFunc
}

func NewBroadcastOrchestrator(srv *Server) *BroadcastOrchestrator {
	return &BroadcastOrchestrator{
		BroadcastHandler:    srv.broadcastSrv.broadcastHandler,
		ForwardHandler:      srv.broadcastSrv.forwardHandler,
		SendToClientHandler: srv.broadcastSrv.sendToClientHandler,
	}
}

type BroadcastOption func(*BroadcastOptions)

func WithSubset(srvAddrs ...string) BroadcastOption {
	return func(b *BroadcastOptions) {
		b.ServerAddresses = srvAddrs
	}
}

func WithGossip(percentage float32, ttl int) BroadcastOption {
	return func(b *BroadcastOptions) {
		b.GossipPercentage = percentage
		b.TTL = ttl
	}
}

func WithTTL(ttl int) BroadcastOption {
	return func(b *BroadcastOptions) {
		b.TTL = ttl
	}
}

func WithDeadline(deadline time.Time) BroadcastOption {
	return func(b *BroadcastOptions) {
		b.Deadline = deadline
	}
}

func WithoutSelf() BroadcastOption {
	return func(b *BroadcastOptions) {
		b.SkipSelf = true
	}
}

func WithoutUniquenessChecks() BroadcastOption {
	return func(b *BroadcastOptions) {
		b.OmitUniquenessChecks = true
	}
}

type BroadcastOptions struct {
	ServerAddresses      []string
	GossipPercentage     float32
	TTL                  int
	Deadline             time.Time
	OmitUniquenessChecks bool
	SkipSelf             bool
}

func NewBroadcastOptions() BroadcastOptions {
	return BroadcastOptions{}
}

type Broadcaster interface{}

type BroadcastMetadata struct {
	BroadcastID  string
	SenderType   string // type of sender, could be: Client or Server
	SenderID     uint32 // nodeID of last hop
	SenderAddr   string // address of last hop
	OriginAddr   string // address of the origin
	OriginMethod string // the first method called by the origin
	Method       string // the current method
	Count        uint64 // number of messages received to the current method
	Digest       []byte // digest of original message sent by client
}

func newBroadcastMetadata(md *ordering.Metadata, count uint64) BroadcastMetadata {
	if md == nil {
		return BroadcastMetadata{}
	}
	tmp := strings.Split(md.Method, ".")
	m := ""
	if len(tmp) >= 1 {
		m = tmp[len(tmp)-1]
	}
	return BroadcastMetadata{
		BroadcastID:  md.BroadcastMsg.BroadcastID,
		SenderType:   md.BroadcastMsg.SenderType,
		SenderID:     md.BroadcastMsg.SenderID,
		SenderAddr:   md.BroadcastMsg.SenderAddr,
		OriginAddr:   md.BroadcastMsg.OriginAddr,
		OriginMethod: md.BroadcastMsg.OriginMethod,
		Method:       m,
		Count:        count,
	}
}

type bMsg struct {
	broadcast   bool
	broadcastID string
	msg         *broadcastMsg
	method      string
	reply       *reply
	//receiveChan chan error
}

type broadcastMsg struct {
	request     RequestTypes
	method      string
	broadcastID string
	options     BroadcastOptions
	ctx         context.Context
}

func newBroadcastMessage(broadcastID string, req RequestTypes, method string, options BroadcastOptions) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		broadcastID: broadcastID,
		options:     options,
		ctx:         context.WithValue(context.Background(), BroadcastID, broadcastID),
	}
}

func newBroadcastMessage2(broadcastID string, req RequestTypes, method string) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		broadcastID: broadcastID,
		ctx:         context.WithValue(context.Background(), BroadcastID, broadcastID),
	}
}
