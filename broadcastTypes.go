package gorums

import (
	"context"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type broadcastFunc func(ctx context.Context, req requestTypes, broadcastID string)

type requestTypes interface {
	ProtoReflect() protoreflect.Message
}

type responseTypes interface {
	ProtoReflect() protoreflect.Message
}

type defaultImplementationFunc[T requestTypes, V responseTypes] func(ServerCtx, T) (V, error)
type implementationFunc[T requestTypes, V broadcastStruct] func(ServerCtx, T, V) error

type responseMsg interface {
	getResponse() responseTypes
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
	response    responseTypes
	err         error
	broadcastID string
	timestamp   time.Time
	ttl         time.Duration
	respType    respType
}

func newResponseMessage(response responseTypes, err error, broadcastID string, respType respType, ttl time.Duration) *responseMessage {
	return &responseMessage{
		response:    response,
		err:         err,
		broadcastID: broadcastID,
		timestamp:   time.Now(),
		ttl:         ttl,
		respType:    respType,
	}
}

func (r *responseMessage) getResponse() responseTypes {
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

type broadcastStruct interface {
	getMethod() string
	shouldBroadcast() bool
	shouldReturnToClient() bool
	reset(...string)
	getRequest() requestTypes
	getResponse() responseTypes
	getError() error
	GetBroadcastID() string
}

type BroadcastStruct struct {
	method                  string // could make this a slice to support multiple broadcasts in one gRPC method
	shouldBroadcastVal      bool
	shouldReturnToClientVal bool
	req                     requestTypes // could make this a slice to support multiple broadcasts in one gRPC method
	resp                    responseTypes
	err                     error // part of client response
	broadcastID             string
}

func NewBroadcastStruct() *BroadcastStruct {
	return &BroadcastStruct{}
}

// Only meant for internal use
func (b *BroadcastStruct) SetBroadcastValues(method string, req requestTypes) {
	b.method = method
	b.shouldBroadcastVal = true
	b.req = req
}

// Only meant for internal use
func (b *BroadcastStruct) SetReturnToClient(resp responseTypes, err error) {
	b.method = "client"
	b.shouldReturnToClientVal = true
	b.resp = resp
	b.err = err
}

func (b *BroadcastStruct) getMethod() string {
	return b.method
}
func (b *BroadcastStruct) getRequest() requestTypes {
	return b.req
}
func (b *BroadcastStruct) getResponse() responseTypes {
	return b.resp
}
func (b *BroadcastStruct) shouldBroadcast() bool {
	return b.shouldBroadcastVal
}
func (b *BroadcastStruct) shouldReturnToClient() bool {
	return b.shouldReturnToClientVal
}
func (b *BroadcastStruct) getError() error {
	return b.err
}
func (b *BroadcastStruct) GetBroadcastID() string {
	return b.broadcastID
}
func (b *BroadcastStruct) reset(broadcastID ...string) {
	b.method = ""
	b.shouldBroadcastVal = false
	b.shouldReturnToClientVal = false
	b.req = nil
	b.resp = nil
	b.err = nil
	if len(broadcastID) >= 1 {
		b.broadcastID = broadcastID[0]
	} else {
		b.broadcastID = ""
	}
}
