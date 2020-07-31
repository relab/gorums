package gorums

import (
	"context"
	"sync"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

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
	Manager        *Manager
	Nodes          []*Node
	Message        protoreflect.ProtoMessage
	MethodID       int32
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, int, bool)
	ServerStream   bool
}

func CorrectableCall(ctx context.Context, d CorrectableCallData) *Correctable {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *orderingResult, len(d.Nodes))
	d.Manager.putChan(msgID, replyChan)

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	expected := len(d.Nodes)
	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				expected--
				continue
			}
		}
		n.sendQ <- &Message{Metadata: md, Message: msg}
	}

	corr := &Correctable{donech: make(chan struct{}, 1)}

	go func() {
		defer d.Manager.deleteChan(msgID)

		var (
			resp    protoreflect.ProtoMessage
			errs    []GRPCError
			rlevel  int
			clevel  = LevelNotSet
			quorum  bool
			replies = make(map[uint32]protoreflect.ProtoMessage)
		)

		for {
			select {
			case r := <-replyChan:
				if r.err != nil {
					errs = append(errs, GRPCError{r.nid, r.err})
					break
				}
				reply := r.reply
				replies[r.nid] = reply
				if resp, rlevel, quorum = d.QuorumFunction(d.Message, replies); quorum {
					if quorum {
						corr.set(reply, rlevel, nil, true)
						return
					}
					if rlevel > clevel {
						clevel = rlevel
						corr.set(reply, rlevel, nil, false)
					}
				}
			case <-ctx.Done():
				corr.set(resp, clevel, QuorumCallError{"incomplete call", len(replies), errs}, true)
				return
			}
			if (d.ServerStream && len(errs) == expected) || (!d.ServerStream && len(errs)+len(replies) == expected) {
				corr.set(resp, clevel, QuorumCallError{"incomplete call", len(replies), errs}, true)
				return
			}
		}
	}()

	return corr
}
