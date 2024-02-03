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
	getMethods() []string
	shouldBroadcast() bool
	shouldReturnToClient() bool
	reset(broadcastID string)
	getRequest(i int) requestTypes
	getResponses() []responseTypes
	getError(i int) error
	GetBroadcastID() string
}

type BroadcastStruct struct {
	methods                 []string // could make this a slice to support multiple broadcasts in one gRPC method
	shouldBroadcastVal      bool
	shouldReturnToClientVal bool
	reqs                    []requestTypes // could make this a slice to support multiple broadcasts in one gRPC method
	resps                   []responseTypes
	errs                    []error // part of client response
	broadcastID             string
}

func NewBroadcastStruct() *BroadcastStruct {
	return &BroadcastStruct{}
}

// This method should be used by generated code only.
func (b *BroadcastStruct) SetBroadcastValues(method string, req requestTypes) {
	b.methods = append(b.methods, method)
	b.shouldBroadcastVal = true
	b.reqs = append(b.reqs, req)
}

// This method should be used by generated code only.
func (b *BroadcastStruct) SetReturnToClient(resp responseTypes, err error) {
	b.shouldReturnToClientVal = true
	b.resps = append(b.resps, resp)
	b.errs = append(b.errs, err)
}

func (b *BroadcastStruct) getMethods() []string {
	return b.methods
}
func (b *BroadcastStruct) getRequest(i int) requestTypes {
	if i >= len(b.reqs) {
		panic("inconsistent requests and methods in broadcast")
	}
	return b.reqs[i]
}
func (b *BroadcastStruct) getResponses() []responseTypes {
	return b.resps
}
func (b *BroadcastStruct) shouldBroadcast() bool {
	return b.shouldBroadcastVal
}
func (b *BroadcastStruct) shouldReturnToClient() bool {
	return b.shouldReturnToClientVal
}
func (b *BroadcastStruct) getError(i int) error {
	if i >= len(b.errs) {
		panic("inconsistent errors and responses in return to client")
	}
	return b.errs[i]
}
func (b *BroadcastStruct) GetBroadcastID() string {
	return b.broadcastID
}
func (b *BroadcastStruct) reset(broadcastID string) {
	b.methods = make([]string, 0)
	b.shouldBroadcastVal = false
	b.shouldReturnToClientVal = false
	b.reqs = make([]requestTypes, 0)
	b.resps = make([]responseTypes, 0)
	b.errs = make([]error, 0)
	b.broadcastID = broadcastID
	/*if len(broadcastID) >= 1 {
		b.broadcastID = broadcastID[0]
	} else {
		b.broadcastID = ""
	}*/
}
