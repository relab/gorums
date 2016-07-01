package dev

import (
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type readReply struct {
	nid   uint32
	reply *State
	err   error
}

func (m *Manager) read(c *Configuration, args *ReadRequest) (*ReadReply, error) {
	var (
		replyChan   = make(chan readReply, c.Size())
		stopSignal  = make(chan struct{})
		replyValues = make([]*State, 0, c.quorum)
		errCount    int
		quorum      bool
		reply       = &ReadReply{NodeIDs: make([]uint32, 0, c.quorum)}
		ctx, cancel = context.WithCancel(context.Background())
	)

	for _, n := range c.nodes {
		go func(node *Node) {
			reply := new(State)
			ce := make(chan error, 1)
			start := time.Now()
			go func() {
				select {
				case ce <- grpc.Invoke(
					ctx,
					"/dev.Register/Read",
					args,
					reply,
					node.conn,
				):
				case <-stopSignal:
					return
				}
			}()
			select {
			case err := <-ce:
				switch grpc.Code(err) { // nil -> codes.OK
				case codes.OK, codes.Canceled:
					node.setLatency(time.Since(start))
				default:
					node.setLastErr(err)
				}
				replyChan <- readReply{node.id, reply, err}
			case <-stopSignal:
				return
			}
		}(n)
	}

	defer close(stopSignal)
	defer cancel()

	/*
		Alternative for time.After in select below: stop rpc timeout timer explicitly.

		See
		https://github.com/kubernetes/kubernetes/pull/23210/commits/e4b369e1d74ac8f2d2a20afce92d93c804afa5d2
		and
		https://github.com/golang/go/issues/8898l

		t := time.NewTimer(c.timeout)
		defer t.Stop()

		and change the corresponding select case below:

		case <-t.C:

		Actually gaven an +1% on the local read benchmark, so not implemted yet.
	*/

	for {

		select {
		case r := <-replyChan:
			if r.err != nil {
				errCount++
				goto terminationCheck
			}
			replyValues = append(replyValues, r.reply)
			reply.NodeIDs = append(reply.NodeIDs, r.nid)
			if reply.Reply, quorum = m.readqf(c, replyValues); quorum {
				return reply, nil
			}
		case <-time.After(c.timeout):
			return reply, TimeoutRPCError{c.timeout, errCount, len(replyValues)}
		}

	terminationCheck:
		if errCount+len(replyValues) == c.Size() {
			return reply, IncompleteRPCError{errCount, len(replyValues)}
		}

	}
}

type writeReply struct {
	nid   uint32
	reply *WriteResponse
	err   error
}

func (m *Manager) write(c *Configuration, args *State) (*WriteReply, error) {
	var (
		replyChan   = make(chan writeReply, c.Size())
		stopSignal  = make(chan struct{})
		replyValues = make([]*WriteResponse, 0, c.quorum)
		errCount    int
		quorum      bool
		reply       = &WriteReply{NodeIDs: make([]uint32, 0, c.quorum)}
		ctx, cancel = context.WithCancel(context.Background())
	)

	for _, n := range c.nodes {
		go func(node *Node) {
			reply := new(WriteResponse)
			ce := make(chan error, 1)
			start := time.Now()
			go func() {
				select {
				case ce <- grpc.Invoke(
					ctx,
					"/dev.Register/Write",
					args,
					reply,
					node.conn,
				):
				case <-stopSignal:
					return
				}
			}()
			select {
			case err := <-ce:
				switch grpc.Code(err) { // nil -> codes.OK
				case codes.OK, codes.Canceled:
					node.setLatency(time.Since(start))
				default:
					node.setLastErr(err)
				}
				replyChan <- writeReply{node.id, reply, err}
			case <-stopSignal:
				return
			}
		}(n)
	}

	defer close(stopSignal)
	defer cancel()

	for {

		select {
		case r := <-replyChan:
			if r.err != nil {
				errCount++
				goto terminationCheck
			}
			replyValues = append(replyValues, r.reply)
			reply.NodeIDs = append(reply.NodeIDs, r.nid)
			if reply.Reply, quorum = m.writeqf(c, replyValues); quorum {
				return reply, nil
			}
		case <-time.After(c.timeout):
			return reply, TimeoutRPCError{c.timeout, errCount, len(replyValues)}
		}

	terminationCheck:
		if errCount+len(replyValues) == c.Size() {
			return reply, IncompleteRPCError{errCount, len(replyValues)}
		}
	}
}

func (m *Manager) writeAsync(c *Configuration, args *State) error {
	for _, node := range c.nodes {
		go func(nodeID uint32) {
			stream := m.writeAsyncClients[nodeID]
			if stream == nil {
				panic("execeptional: node client stream not found")
			}
			err := stream.Send(args)
			if err == nil {
				return
			}
			if m.logger != nil {
				m.logger.Printf("%d: writeAsync stream send error: %v", nodeID, err)
			}
		}(node.id)
	}

	return nil
}
