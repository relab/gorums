package gorums

import (
	"context"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

type watcher struct {
	level int
	ch    chan struct{}
}

type Correctable struct {
	mu       sync.Mutex
	reply    protoreflect.ProtoMessage
	level    int
	err      error
	done     bool
	watchers []*watcher
	donech   chan struct{}
}

func (c *Correctable) Get() (protoreflect.ProtoMessage, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reply, c.level, c.err
}

func (c *Correctable) Done() <-chan struct{} {
	return c.donech
}

func (c *Correctable) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
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

type CorrectableCallData struct {
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool)
	ServerStream   bool
}

func (c Configuration) CorrectableCall(ctx context.Context, d CorrectableCallData, opts ...CallOption) *Correctable {
	o := getCallOptions(E_Correctable, opts)
	expectedReplies := len(c)
	md := c.newCall(d.Method)
	replyChan, callDone := c.newReply(md, expectedReplies)

	channels := o.getChannels(c)
	for i, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		channels[i].sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}
	}

	corr := &Correctable{donech: make(chan struct{}, 1)}

	go func() {
		defer callDone()

		var (
			resp    protoreflect.ProtoMessage
			errs    []Error
			rlevel  int
			clevel  = LevelNotSet
			quorum  bool
			replies = make(map[uint32]protoreflect.ProtoMessage)
		)

		for {
			select {
			case r := <-replyChan:
				if r.err != nil {
					errs = append(errs, Error{r.nid, r.err})
					break
				}
				replies[r.nid] = r.msg
				if resp, rlevel, quorum = d.QuorumFunction(d.Message, replies); quorum {
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
				corr.set(resp, clevel, ctx.Err(), true)
				return
			}
			if (d.ServerStream && len(errs) == expectedReplies) || (!d.ServerStream && len(errs)+len(replies) == expectedReplies) {
				corr.set(resp, clevel, QuorumCallError{"incomplete call", len(replies), errs}, true)
				return
			}
		}
	}()

	return corr
}
