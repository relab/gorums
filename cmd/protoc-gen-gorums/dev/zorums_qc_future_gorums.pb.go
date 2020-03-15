// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	context "context"
	trace "golang.org/x/net/trace"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	time "time"
)

// ReadQuorumCallFuture asynchronously invokes a quorum call on configuration c
// and returns a FutureReadResponse, which can be used to inspect the quorum call
// reply and error when available.
func (c *Configuration) ReadQuorumCallFuture(ctx context.Context, in *ReadRequest, opts ...grpc.CallOption) *FutureReadResponse {
	fut := &FutureReadResponse{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	go func() {
		defer close(fut.c)
		c.readQuorumCallFuture(ctx, in, fut, opts...)
	}()
	return fut
}

// Get returns the reply and any error associated with the ReadQuorumCallFuture.
// The method blocks until a reply or error is available.
func (f *FutureReadResponse) Get() (*ReadResponse, error) {
	<-f.c
	return f.ReadResponse, f.err
}

// Done reports if a reply and/or error is available for the ReadQuorumCallFuture.
func (f *FutureReadResponse) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

func (c *Configuration) readQuorumCallFuture(ctx context.Context, in *ReadRequest, resp *FutureReadResponse, opts ...grpc.CallOption) {
	var ti traceInfo
	if c.mgr.opts.trace {
		ti.Trace = trace.New("gorums."+c.tstring()+".Sent", "ReadQuorumCallFuture")
		defer ti.Finish()

		ti.firstLine.cid = c.id
		if deadline, ok := ctx.Deadline(); ok {
			ti.firstLine.deadline = time.Until(deadline)
		}
		ti.LazyLog(&ti.firstLine, false)
		ti.LazyLog(&payload{sent: true, msg: in}, false)

		defer func() {
			ti.LazyLog(&qcresult{ids: resp.NodeIDs, reply: resp.ReadResponse, err: resp.err}, false)
			if resp.err != nil {
				ti.SetError()
			}
		}()
	}

	expected := c.n
	replyChan := make(chan internalReadResponse, expected)
	for _, n := range c.nodes {
		go callGRPCReadQuorumCallFuture(ctx, n, in, replyChan)
	}

	var (
		replyValues = make([]*ReadResponse, 0, c.n)
		reply       *ReadResponse
		errs        []GRPCError
		quorum      bool
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = append(resp.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}

			if c.mgr.opts.trace {
				ti.LazyLog(&payload{sent: false, id: r.nid, msg: r.reply}, false)
			}

			replyValues = append(replyValues, r.reply)
			if reply, quorum = c.qspec.ReadQuorumCallFutureQF(replyValues); quorum {
				resp.ReadResponse, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.ReadResponse, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
			return
		}
		if len(errs)+len(replyValues) == expected {
			resp.ReadResponse, resp.err = reply, QuorumCallError{"incomplete call", len(replyValues), errs}
			return
		}
	}
}

func callGRPCReadQuorumCallFuture(ctx context.Context, node *Node, in *ReadRequest, replyChan chan<- internalReadResponse) {
	reply := new(ReadResponse)
	start := time.Now()
	err := node.conn.Invoke(ctx, "/dev.ReaderService/ReadQuorumCallFuture", in, reply)
	s, ok := status.FromError(err)
	if ok && (s.Code() == codes.OK || s.Code() == codes.Canceled) {
		node.setLatency(time.Since(start))
	} else {
		node.setLastErr(err)
	}
	replyChan <- internalReadResponse{node.id, reply, err}
}