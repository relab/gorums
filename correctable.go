package gorums

import (
	"context"
	"sync"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

type watcher struct {
	level int
	ch    chan struct{}
}

// Correctable encapsulates the state of a correctable quorum call.
//
// This struct should be used by generated code only.
type Correctable struct {
	mu       sync.Mutex
	reply    protoreflect.ProtoMessage
	level    int
	err      error
	done     bool
	watchers []*watcher
	donech   chan struct{}
}

// Get returns the latest response, the current level, and the last error.
func (c *Correctable) Get() (protoreflect.ProtoMessage, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reply, c.level, c.err
}

// Done returns a channel that will close when the correctable call is completed.
func (c *Correctable) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will close when the correctable call has reached a specified level.
func (c *Correctable) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level <= c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &watcher{level, ch})
	return ch
}

func (c *Correctable) set(reply protoreflect.ProtoMessage, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.reply, c.level, c.err, c.done = reply, level, err, done
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

// CorrectableCallData contains data for making a correctable quorum call.
//
// This struct should only be used by generated code.
type CorrectableCallData struct {
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool)
	ServerStream   bool
}

type correctableCallState struct {
	md              *ordering.Metadata
	data            CorrectableCallData
	replyChan       <-chan response
	expectedReplies int
}

// CorrectableCall starts a new correctable quorum call and returns a new Correctable object.
//
// This method should only be used by generated code.
func (c RawConfiguration) CorrectableCall(ctx context.Context, d CorrectableCallData) *Correctable {
	expectedReplies := len(c)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)

	replyChan := make(chan response, expectedReplies)
	for _, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, streaming: d.ServerStream}, replyChan)
	}

	corr := &Correctable{donech: make(chan struct{}, 1)}

	go c.handleCorrectableCall(ctx, corr, correctableCallState{
		md:              md,
		data:            d,
		replyChan:       replyChan,
		expectedReplies: expectedReplies,
	})

	return corr
}

func (c RawConfiguration) handleCorrectableCall(ctx context.Context, corr *Correctable, state correctableCallState) {
	var (
		resp    protoreflect.ProtoMessage
		errs    []nodeError
		rlevel  int
		clevel  = LevelNotSet
		quorum  bool
		replies = make(map[uint32]protoreflect.ProtoMessage)
	)

	if state.data.ServerStream {
		for _, n := range c {
			defer n.channel.deleteRouter(state.md.GetMessageID())
		}
	}

	for {
		select {
		case r := <-state.replyChan:
			if r.err != nil {
				errs = append(errs, nodeError{nodeID: r.nid, cause: r.err})
				break
			}
			replies[r.nid] = r.msg
			if resp, rlevel, quorum = state.data.QuorumFunction(state.data.Message, replies); quorum {
				if quorum {
					corr.set(r.msg, rlevel, nil, true)
					return
				}
				if rlevel > clevel {
					clevel = rlevel
					corr.set(r.msg, rlevel, nil, false)
				}
			}
		case <-ctx.Done():
			corr.set(resp, clevel, QuorumCallError{cause: ctx.Err(), errors: errs, replies: len(replies)}, true)
			return
		}
		if (state.data.ServerStream && len(errs) == state.expectedReplies) ||
			(!state.data.ServerStream && len(errs)+len(replies) == state.expectedReplies) {
			corr.set(resp, clevel, QuorumCallError{cause: Incomplete, errors: errs, replies: len(replies)}, true)
			return
		}
	}
}
