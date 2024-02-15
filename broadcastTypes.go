package gorums

import (
	"context"
	"strings"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastFunc func(ctx context.Context, req RequestTypes, broadcastMetadata BroadcastMetadata, srvAddrs []string)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastHandlerFunc func(method string, req RequestTypes, metadata BroadcastMetadata, data ...BroadcastOptions)
type BroadcastReturnToClientHandlerFunc func(resp ResponseTypes, err error, metadata BroadcastMetadata)

type defaultImplementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)

type implementationFuncB[T RequestTypes, V IBroadcastStruct] func(ServerCtx, T, V)

type responseMsg interface {
	getResponse() ResponseTypes
	getError() error
	getBroadcastID() string
	valid() bool
	getType() respType
}

type respType int

const (
	unhandled respType = iota
	clientResponse
	timeout
	done
)

type responseMessage struct {
	response    ResponseTypes
	err         error
	broadcastID string
	timestamp   time.Time
	ttl         time.Duration
	respType    respType
}

func newResponseMessage(response ResponseTypes, err error, broadcastID string, respType respType, ttl time.Duration) *responseMessage {
	return &responseMessage{
		response:    response,
		err:         err,
		broadcastID: broadcastID,
		timestamp:   time.Now(),
		ttl:         ttl,
		respType:    respType,
	}
}

func (r *responseMessage) getResponse() ResponseTypes {
	return r.response
}

func (r *responseMessage) getError() error {
	return r.err
}

func (r *responseMessage) getBroadcastID() string {
	return r.broadcastID
}

func (r *responseMessage) valid() bool {
	return r.respType == clientResponse && time.Since(r.timestamp) <= r.ttl
}

func (r *responseMessage) getType() respType {
	return r.respType
}

type clientRequest struct {
	id       string
	ctx      ServerCtx
	finished chan<- *Message
	metadata *ordering.Metadata
	status   respType
}

type SpBroadcast struct {
	BroadcastHandler      BroadcastHandlerFunc
	ReturnToClientHandler BroadcastReturnToClientHandlerFunc
}

func NewSpBroadcastStruct() *SpBroadcast {
	return &SpBroadcast{}
}

type BroadcastOptions struct {
	ServerAddresses      []string
	GossipPercentage     float32
	OmitUniquenessChecks bool
	SkipSelf             bool
}

type IBroadcastStruct interface {
	setMetadataHandler(func(metadata BroadcastMetadata))
	setMetadata(metadata BroadcastMetadata)
}

type BroadcastStruct struct {
	metadataHandler func(metadata BroadcastMetadata)
}

func (b *BroadcastStruct) setMetadataHandler(handler func(metadata BroadcastMetadata)) {
	b.metadataHandler = handler
}

func (b *BroadcastStruct) setMetadata(metadata BroadcastMetadata) {
	b.metadataHandler(metadata)
}

func NewBroadcastStruct() *BroadcastStruct {
	return &BroadcastStruct{}
}

type BroadcastMetadata struct {
	BroadcastID string
	Sender      string
	SenderID    string
	SenderAddr  string
	OriginID    string
	OriginAddr  string
	Method      string

	PublicKey string
	Signature string
	MAC       string
}

func newBroadcastMetadata(md *ordering.Metadata) BroadcastMetadata {
	tmp := strings.Split(md.Method, ".")
	m := ""
	if len(tmp) >= 1 {
		m = tmp[len(tmp)-1]
	}

	return BroadcastMetadata{
		BroadcastID: md.BroadcastMsg.BroadcastID,
		Sender:      md.BroadcastMsg.Sender,
		SenderID:    md.BroadcastMsg.SenderID,
		SenderAddr:  md.BroadcastMsg.SenderAddr,
		OriginID:    md.BroadcastMsg.OriginID,
		OriginAddr:  md.BroadcastMsg.OriginAddr,
		Method:      m,
		PublicKey:   md.BroadcastMsg.PublicKey,
		Signature:   md.BroadcastMsg.Signature,
		MAC:         md.BroadcastMsg.MAC,
	}
}
