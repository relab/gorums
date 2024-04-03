package gorums

import (
	"context"
	"fmt"
	"log/slog"
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

type broadcastFunc func(ctx context.Context, in RequestTypes, req clientRequest, options BroadcastOptions)

type RequestTypes interface {
	ProtoReflect() protoreflect.Message
}

type ResponseTypes interface {
	ProtoReflect() protoreflect.Message
}

type BroadcastHandlerFunc func(method string, req RequestTypes, broadcastID string, data ...BroadcastOptions)
type BroadcastSendToClientHandlerFunc func(broadcastID string, resp ResponseTypes, err error)

type defaultImplementationFunc[T RequestTypes, V ResponseTypes] func(ServerCtx, T) (V, error)

type implementationFunc[T RequestTypes, V Broadcaster] func(ServerCtx, T, V)

type responseMsg struct {
	response    ResponseTypes
	err         error
	broadcastID string
	timestamp   time.Time
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

type clientRequest struct {
	ctx        ServerCtx
	finished   chan<- *Message
	metadata   *ordering.Metadata
	senderType string
	//methods   []string
	//counts    map[string]uint64
	timestamp time.Time
	doneChan  chan struct{}
}

// The BroadcastOrchestrator is used as a container for all
// broadcast handlers. The BroadcastHandler takes in a method
// and schedules it for broadcasting. SendToClientHandler works
// similarly but it sends the message to the calling client.
//
// It is necessary to use an orchestrator to hide certain
// implementation details, such as internal methods on the
// broadcaster struct. The BroadcastOrchestrator will thus
// be an unimported field in the broadcaster struct in the
// generated code.
type BroadcastOrchestrator struct {
	BroadcastHandler    BroadcastHandlerFunc
	SendToClientHandler BroadcastSendToClientHandlerFunc
}

func NewBroadcastOrchestrator(srv *Server) *BroadcastOrchestrator {
	return &BroadcastOrchestrator{
		BroadcastHandler:    srv.broadcastSrv.broadcasterHandler,
		SendToClientHandler: srv.broadcastSrv.sendToClient,
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

type CacheOption int

/*
redis:

  - noeviction: New values aren’t saved when memory limit is reached. When a database uses replication, this applies to the primary database
  - allkeys-lru: Keeps most recently used keys; removes least recently used (LRU) keys
  - allkeys-lfu: Keeps frequently used keys; removes least frequently used (LFU) keys
  - volatile-lru: Removes least recently used keys with the expire field set to true.
  - volatile-lfu: Removes least frequently used keys with the expire field set to true.
  - allkeys-random: Randomly removes keys to make space for the new data added.
  - volatile-random: Randomly removes keys with expire field set to true.
  - volatile-ttl: Removes keys with expire field set to true and the shortest remaining time-to-live (TTL) value.
*/
const (
	noeviction CacheOption = iota
	allkeysLRU
	allkeysLFU
	volatileLRU
	volatileLFU
	allkeysRANDOM
	volatileRANDOM
	volatileTTL
)

type RequestMap struct {
	data         map[string]clientRequest
	mutex        sync.RWMutex
	handledReqs  map[string]time.Time
	pending      map[string]*responseMsg
	doneChan     chan struct{}
	removeOption CacheOption
}

func NewRequestMap() *RequestMap {
	reqMap := &RequestMap{
		data:        make(map[string]clientRequest, 100),
		handledReqs: make(map[string]time.Time, 100),
		doneChan:    make(chan struct{}),
	}
	return reqMap
}

func (list *RequestMap) cleanup() {
	for {
		select {
		case <-list.doneChan:
			return
		case <-time.After(1 * time.Minute):
		}
		//time.Sleep(3 * time.Second)
		list.mutex.Lock()
		del := make([]string, 0)
		// remove handled reqs
		for broadcastID, timestamp := range list.handledReqs {
			if time.Since(timestamp) > 30*time.Minute {
				del = append(del, broadcastID)
			}
		}
		// remove stale reqs
		for broadcastID, req := range list.data {
			if time.Since(req.timestamp) > 30*time.Minute {
				del = append(del, broadcastID)
			}
		}
		for _, broadcastID := range del {
			delete(list.handledReqs, broadcastID)
			delete(list.data, broadcastID)
		}
		list.mutex.Unlock()
	}
}

func (list *RequestMap) Stop() {
	close(list.doneChan)
}

func (list *RequestMap) Add(identifier string, element clientRequest) (count uint64, err error) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// signal that the request has been handled and is thus completed
	if _, ok := list.handledReqs[identifier]; ok {
		return 0, fmt.Errorf("the request is already handled")
	}
	// only add method if element already exists
	if elem, ok := list.data[identifier]; ok {
		//elem.methods = append(elem.methods, element.metadata.Method)
		// add to count
		//if _, ok := elem.counts[element.metadata.Method]; !ok {
		//	elem.counts[element.metadata.Method] = 1
		//} else {
		//	elem.counts[element.metadata.Method]++
		//}
		//return elem.counts[element.metadata.Method], nil
		old := elem.senderType
		new := element.senderType
		// we receive the original request after is has been broadcasted by another server
		if new == BroadcastClient && old == BroadcastServer {
			elem.senderType = BroadcastClient
			elem.finished = element.finished
			list.data[identifier] = elem
		}
		return 0, nil
	}
	//methods := []string{element.metadata.Method}
	//element.methods = methods
	element.timestamp = time.Now()
	//element.counts = make(map[string]uint64)
	//element.counts[element.metadata.Method] = 1
	list.data[identifier] = element
	//return element.counts[element.metadata.Method], nil
	return 0, nil
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
	_, alive := list.data[identifier]
	_, handled := list.handledReqs[identifier]
	// options:
	//	1. always true if it exists in handledReqs
	//	2. true if it does not exists in any of the maps
	//	3. false if it only exists in data
	return handled || !alive
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
	processingTime := time.Since(elem.timestamp)
	delete(list.data, identifier)
	if processingTime > 3*time.Millisecond {
		slog.Error("processing time", "dur", processingTime)
	}
}

func (list *RequestMap) Reset() {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	del := make([]string, 0, len(list.data))
	// remove client reqs
	//for broadcastID, elem := range list.data {
	for broadcastID := range list.data {
		del = append(del, broadcastID)
		//list.handledReqs[broadcastID] = elem.timestamp
	}
	for _, broadcastID := range del {
		delete(list.data, broadcastID)
	}
}

// Retrieves a client requests with the given broadcastID and returns
// a bool defining whether the request is valid or not.
// The request is considered valid only if the request has not yet
// been handled and it has previously been added to client requests.
func (list *RequestMap) GetIfValid(identifier string) (clientRequest, bool) {
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

func (list *RequestMap) AddToPending(identifier string, resp *responseMsg) error {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	// signal that the request has been handled and is thus completed
	if _, ok := list.data[identifier]; ok {
		return fmt.Errorf("request is not handled yet")
	}
	if _, ok := list.pending[identifier]; ok {
		return fmt.Errorf("request already added to pending")
	}
	list.pending[identifier] = resp
	return nil
}

func (list *RequestMap) GetInPending(identifier string) (*responseMsg, bool) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	resp, ok := list.pending[identifier]
	if ok {
		delete(list.pending, identifier)
	}
	return resp, ok
}

type broadcastMsg struct {
	request     RequestTypes
	method      string
	broadcastID string
	options     BroadcastOptions
	finished    chan<- struct{}
	ctx         context.Context
}

func (b *broadcastMsg) setFinished() {
	close(b.finished)
}

func newBroadcastMessage(broadcastID string, req RequestTypes, method string, options BroadcastOptions, finished chan<- struct{}) *broadcastMsg {
	return &broadcastMsg{
		request:     req,
		method:      method,
		broadcastID: broadcastID,
		options:     options,
		finished:    finished,
		ctx:         context.WithValue(context.Background(), BroadcastID, broadcastID),
	}
}
