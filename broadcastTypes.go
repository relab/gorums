package gorums

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	BroadcastClient string = "client"
	BroadcastServer string = "server"
	BroadcastID     string = "broadcastID"
)

type serverView interface {
	broadcastCall(ctx context.Context, d broadcastCallData)
}

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

type implementationFuncB[T RequestTypes, V iBroadcastStruct] func(ServerCtx, T, V)

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
	handled  bool
}

type SpBroadcast struct {
	BroadcastHandler      BroadcastHandlerFunc
	ReturnToClientHandler BroadcastReturnToClientHandlerFunc
}

func NewSpBroadcastStruct() *SpBroadcast {
	return &SpBroadcast{}
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

type iBroadcastStruct interface {
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
	BroadcastID  string
	Sender       string
	SenderID     string
	SenderAddr   string
	OriginID     string
	OriginAddr   string
	OriginMethod string
	Method       string
}

func newBroadcastMetadata(md *ordering.Metadata) BroadcastMetadata {
	tmp := strings.Split(md.Method, ".")
	m := ""
	if len(tmp) >= 1 {
		m = tmp[len(tmp)-1]
	}

	return BroadcastMetadata{
		BroadcastID:  md.BroadcastMsg.BroadcastID,
		Sender:       md.BroadcastMsg.Sender,
		SenderID:     md.BroadcastMsg.SenderID,
		SenderAddr:   md.BroadcastMsg.SenderAddr,
		OriginID:     md.BroadcastMsg.OriginID,
		OriginAddr:   md.BroadcastMsg.OriginAddr,
		OriginMethod: md.BroadcastMsg.OriginMethod,
		Method:       m,
	}
}

type RequestMap struct {
	data  map[string]clientRequest
	mutex sync.RWMutex
}

func NewRequestMap() *RequestMap {
	return &RequestMap{
		data: make(map[string]clientRequest),
	}
}

func (list *RequestMap) Add(identifier string, element clientRequest) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// do nothing if element already exists
	if _, ok := list.data[identifier]; ok {
		return
	}
	list.data[identifier] = element
}

func (list *RequestMap) Get(identifier string) (clientRequest, bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	elem, ok := list.data[identifier]
	return elem, ok
}

func (list *RequestMap) Set(identifier string, elem clientRequest) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	list.data[identifier] = elem
}

func (list *RequestMap) GetSet(identifier string) (clientRequest, bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	elem, ok := list.data[identifier]
	if !ok {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		// already handled and can be removed?
		return elem, true
	}
	if elem.handled {
		return elem, true
	}
	elem.handled = true
	list.data[identifier] = elem
	return elem, false
}
