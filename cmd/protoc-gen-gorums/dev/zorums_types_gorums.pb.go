// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	empty "github.com/golang/protobuf/ptypes/empty"
	ordering "github.com/relab/gorums/ordering"
	sync "sync"
)

const hasOrderingMethods = true

const multicastMethodID int32 = 0
const multicastPerNodeArgMethodID int32 = 1
const multicast2MethodID int32 = 2
const multicast3MethodID int32 = 3
const multicast4MethodID int32 = 4
const orderingQCMethodID int32 = 5
const orderingPerNodeArgMethodID int32 = 6
const orderingCustomReturnTypeMethodID int32 = 7
const orderingComboMethodID int32 = 8
const orderingUnaryRPCMethodID int32 = 9
const orderingFutureMethodID int32 = 10
const orderingFuturePerNodeArgMethodID int32 = 11
const orderingFutureCustomReturnTypeMethodID int32 = 12
const orderingFutureComboMethodID int32 = 13
const unicastMethodID int32 = 14
const unicast2MethodID int32 = 15

var orderingMethods = map[int32]methodInfo{

	0:  {oneway: true, concurrent: false},
	1:  {oneway: true, concurrent: false},
	2:  {oneway: true, concurrent: false},
	3:  {oneway: true, concurrent: false},
	4:  {oneway: true, concurrent: false},
	5:  {oneway: false, concurrent: false},
	6:  {oneway: false, concurrent: false},
	7:  {oneway: false, concurrent: false},
	8:  {oneway: false, concurrent: false},
	9:  {oneway: false, concurrent: false},
	10: {oneway: false, concurrent: false},
	11: {oneway: false, concurrent: false},
	12: {oneway: false, concurrent: false},
	13: {oneway: false, concurrent: false},
	14: {oneway: true, concurrent: false},
	15: {oneway: true, concurrent: false},
}

type internalEmpty struct {
	nid   uint32
	reply *empty.Empty
	err   error
}

type internalResponse struct {
	nid   uint32
	reply *Response
	err   error
}

// FutureEmpty is a future object for processing replies.
type FutureEmpty struct {
	// the actual reply
	*empty.Empty
	NodeIDs []uint32
	err     error
	c       chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureEmpty) Get() (*empty.Empty, error) {
	<-f.c
	return f.Empty, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *FutureEmpty) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// FutureMyResponse is a future object for processing replies.
type FutureMyResponse struct {
	// the actual reply
	*MyResponse
	NodeIDs []uint32
	err     error
	c       chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureMyResponse) Get() (*MyResponse, error) {
	<-f.c
	return f.MyResponse, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *FutureMyResponse) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// FutureResponse is a future object for processing replies.
type FutureResponse struct {
	// the actual reply
	*Response
	NodeIDs []uint32
	err     error
	c       chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureResponse) Get() (*Response, error) {
	<-f.c
	return f.Response, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *FutureResponse) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// CorrectableEmpty is a correctable object for processing replies.
type CorrectableEmpty struct {
	mu sync.Mutex
	// the actual reply
	*empty.Empty
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableEmpty) Get() (*empty.Empty, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Empty, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableEmpty) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableEmpty) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableEmpty) set(reply *empty.Empty, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.Empty, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// CorrectableMyResponse is a correctable object for processing replies.
type CorrectableMyResponse struct {
	mu sync.Mutex
	// the actual reply
	*MyResponse
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableMyResponse) Get() (*MyResponse, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.MyResponse, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableMyResponse) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableMyResponse) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableMyResponse) set(reply *MyResponse, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.MyResponse, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// CorrectableResponse is a correctable object for processing replies.
type CorrectableResponse struct {
	mu sync.Mutex
	// the actual reply
	*Response
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableResponse) Get() (*Response, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Response, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableResponse) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableResponse) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableResponse) set(reply *Response, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.Response, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// CorrectableStreamEmpty is a correctable object for processing replies.
type CorrectableStreamEmpty struct {
	mu sync.Mutex
	// the actual reply
	*empty.Empty
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamEmpty) Get() (*empty.Empty, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Empty, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableStreamEmpty) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableStreamEmpty) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableStreamEmpty) set(reply *empty.Empty, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.Empty, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// CorrectableStreamMyResponse is a correctable object for processing replies.
type CorrectableStreamMyResponse struct {
	mu sync.Mutex
	// the actual reply
	*MyResponse
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamMyResponse) Get() (*MyResponse, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.MyResponse, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableStreamMyResponse) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableStreamMyResponse) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableStreamMyResponse) set(reply *MyResponse, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.MyResponse, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// CorrectableStreamResponse is a correctable object for processing replies.
type CorrectableStreamResponse struct {
	mu sync.Mutex
	// the actual reply
	*Response
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level int
		ch    chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamResponse) Get() (*Response, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Response, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *CorrectableStreamResponse) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *CorrectableStreamResponse) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *CorrectableStreamResponse) set(reply *Response, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.Response, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}

// ZorumsService is the server-side API for the ZorumsService Service
type ZorumsService interface {
	Multicast(*Request)
	MulticastPerNodeArg(*Request)
	Multicast2(*Request)
	Multicast3(*Request)
	Multicast4(*empty.Empty)
	OrderingQC(*Request) *Response
	OrderingPerNodeArg(*Request) *Response
	OrderingCustomReturnType(*Request) *Response
	OrderingCombo(*Request) *Response
	OrderingUnaryRPC(*Request) *Response
	OrderingFuture(*Request) *Response
	OrderingFuturePerNodeArg(*Request) *Response
	OrderingFutureCustomReturnType(*Request) *Response
	OrderingFutureCombo(*Request) *Response
	Unicast(*Request)
	Unicast2(*Request)
}

func (s *GorumsServer) RegisterZorumsServiceServer(srv ZorumsService) {
	s.srv.handlers[multicastMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Multicast(req)
		return nil
	}
	s.srv.handlers[multicastPerNodeArgMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.MulticastPerNodeArg(req)
		return nil
	}
	s.srv.handlers[multicast2MethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Multicast2(req)
		return nil
	}
	s.srv.handlers[multicast3MethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Multicast3(req)
		return nil
	}
	s.srv.handlers[multicast4MethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(empty.Empty)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Multicast4(req)
		return nil
	}
	s.srv.handlers[orderingQCMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingQCMethodID, ID: in.ID}
		}
		resp := srv.OrderingQC(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingQCMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingPerNodeArgMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingPerNodeArgMethodID, ID: in.ID}
		}
		resp := srv.OrderingPerNodeArg(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingPerNodeArgMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingCustomReturnTypeMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingCustomReturnTypeMethodID, ID: in.ID}
		}
		resp := srv.OrderingCustomReturnType(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingCustomReturnTypeMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingComboMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingComboMethodID, ID: in.ID}
		}
		resp := srv.OrderingCombo(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingComboMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingUnaryRPCMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingUnaryRPCMethodID, ID: in.ID}
		}
		resp := srv.OrderingUnaryRPC(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingUnaryRPCMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingFutureMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingFutureMethodID, ID: in.ID}
		}
		resp := srv.OrderingFuture(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingFutureMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingFuturePerNodeArgMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingFuturePerNodeArgMethodID, ID: in.ID}
		}
		resp := srv.OrderingFuturePerNodeArg(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingFuturePerNodeArgMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingFutureCustomReturnTypeMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingFutureCustomReturnTypeMethodID, ID: in.ID}
		}
		resp := srv.OrderingFutureCustomReturnType(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingFutureCustomReturnTypeMethodID, ID: in.ID}
	}
	s.srv.handlers[orderingFutureComboMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		// TODO: how to handle marshaling errors here
		if err != nil {
			return &ordering.Message{MethodID: orderingFutureComboMethodID, ID: in.ID}
		}
		resp := srv.OrderingFutureCombo(req)
		data, err := s.srv.marshaler.Marshal(resp)
		if err != nil {
			return new(ordering.Message)
		}
		return &ordering.Message{Data: data, MethodID: orderingFutureComboMethodID, ID: in.ID}
	}
	s.srv.handlers[unicastMethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Unicast(req)
		return nil
	}
	s.srv.handlers[unicast2MethodID] = func(in *ordering.Message) *ordering.Message {
		req := new(Request)
		err := s.srv.unmarshaler.Unmarshal(in.GetData(), req)
		if err != nil {
			return nil
		}
		srv.Unicast2(req)
		return nil
	}
}
