package gorums

import (
	"context"
	"net"
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

type implementationFunc[T RequestTypes, V broadcaster] func(ServerCtx, T, V)

type responseMsg struct {
	response    ResponseTypes
	err         error
	broadcastID string
	timestamp   time.Time
	ttl         time.Duration
}

func newResponseMessage(response ResponseTypes, err error, broadcastID string) *responseMsg {
	return &responseMsg{
		response:    response,
		err:         err,
		broadcastID: broadcastID,
		timestamp:   time.Now(),
	}
}

func (r *responseMsg) getResponse() ResponseTypes {
	return r.response
}

func (r *responseMsg) getError() error {
	return r.err
}

func (r *responseMsg) getBroadcastID() string {
	return r.broadcastID
}

func (r *responseMsg) valid() bool {
	if r.ttl != 0 {
		return time.Since(r.timestamp) <= r.ttl
	}
	return true
}

type clientRequest struct {
	id        string
	ctx       ServerCtx
	finished  chan<- *Message
	metadata  *ordering.Metadata
	methods   []string
	timestamp time.Time
	doneChan  chan struct{}
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

// not sure if this is necessary because the implementer
// can decide to run the broadcast in a go routine.
func WithoutWaiting() BroadcastOption {
	return func(b *BroadcastOptions) {}
}

// returns a listener for the given address.
// panics upon errors.
func WithListener(listenAddr string) net.Listener {
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}
	return lis
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

type broadcaster interface {
	setMetadataHandler(func(metadata BroadcastMetadata))
	setMetadata(metadata BroadcastMetadata)
}

type Broadcaster struct {
	metadataHandler func(metadata BroadcastMetadata)
}

func (b *Broadcaster) setMetadataHandler(handler func(metadata BroadcastMetadata)) {
	b.metadataHandler = handler
}

func (b *Broadcaster) setMetadata(metadata BroadcastMetadata) {
	b.metadataHandler(metadata)
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{}
}

type BroadcastMetadata struct {
	BroadcastID  string
	SequenceNo   uint32
	SenderType   string
	SenderAddr   string
	OriginAddr   string
	OriginMethod string
	Deadline     uint64
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
		SequenceNo:   md.BroadcastMsg.SequenceNo,
		SenderType:   md.BroadcastMsg.SenderType,
		SenderAddr:   md.BroadcastMsg.SenderAddr,
		OriginAddr:   md.BroadcastMsg.OriginAddr,
		OriginMethod: md.BroadcastMsg.OriginMethod,
		Deadline:     md.BroadcastMsg.Deadline,
		Method:       m,
	}
}

type RequestMap struct {
	data        map[string]clientRequest
	mutex       sync.RWMutex
	handledReqs map[string]time.Time
}

func NewRequestMap() *RequestMap {
	reqMap := &RequestMap{
		data:        make(map[string]clientRequest),
		handledReqs: make(map[string]time.Time),
	}
	go reqMap.cleanup()
	return reqMap
}

func (list *RequestMap) cleanup() {
	for {
		time.Sleep(1 * time.Minute)
		del := make([]string, 0)
		// remove handled reqs
		for broadcastID, timestamp := range list.handledReqs {
			if time.Now().Sub(timestamp) > 30*time.Minute {
				del = append(del, broadcastID)
			}
		}
		// remove stale reqs
		for broadcastID, req := range list.data {
			if time.Now().Sub(req.timestamp) > 30*time.Minute {
				del = append(del, broadcastID)
			}
		}
		for _, broadcastID := range del {
			delete(list.handledReqs, broadcastID)
			delete(list.data, broadcastID)
		}
	}
}

func (list *RequestMap) Add(identifier string, element clientRequest) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// do nothing if element already exists
	if elem, ok := list.data[identifier]; ok {
		elem.methods = append(elem.methods, element.metadata.Method)
		return
	}
	methods := []string{element.metadata.Method}
	element.methods = methods
	element.timestamp = time.Now()
	list.data[identifier] = element
}

func (list *RequestMap) Get(identifier string) (clientRequest, bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	elem, ok := list.data[identifier]
	return elem, ok
}

func (list *RequestMap) IsHandled(identifier string) bool {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	_, ok := list.handledReqs[identifier]
	return ok
}

func (list *RequestMap) Remove(identifier string) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// do nothing if the element does not exists
	elem, ok := list.data[identifier]
	if !ok {
		return
	}
	// add broadcastID to handled requests.
	// requests added to this map will be
	// removed after 30 minutes by default.
	// the implementer can also provide a
	// custom time to live for a request.
	list.handledReqs[identifier] = elem.timestamp
	delete(list.data, identifier)
}

// Retrieves a client requests with the given broadcastID and returns
// a bool defining whether the request is valid or not.
// The request is considered valid only if the request has not yet
// been handled and it has previously been added to client requests.
func (list *RequestMap) GetStrict(identifier string) (clientRequest, bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	var req clientRequest
	_, ok := list.handledReqs[identifier]
	if ok {
		return req, false
	}
	req, ok = list.data[identifier]
	if !ok {
		// this server has not received a request directly from a client
		// hence, the response should be ignored
		// already handled and can be removed?
		return req, false
	}
	return req, true

}

type broadcastMsg struct {
	from        string
	request     RequestTypes
	method      string
	metadata    BroadcastMetadata
	broadcastID string
	srvAddrs    []string
	finished    chan<- struct{}
	ctx         context.Context
}

func (b *broadcastMsg) setFinished() {
	close(b.finished)
}

func newBroadcastMessage(metadata BroadcastMetadata, req RequestTypes, method, broadcastID string, srvAddrs []string, finished chan<- struct{}) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		metadata:    metadata,
		broadcastID: broadcastID,
		srvAddrs:    srvAddrs,
		finished:    finished,
		ctx:         context.WithValue(context.Background(), BroadcastID, metadata.BroadcastID),
	}
}
