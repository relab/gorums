// Code generated by github.com/relab/gorums/cmd/gentemplates. DO NOT EDIT.
// Template source files to edit is in the 'dev' folder.

package gorums

const calltype_common_definitions_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}
{{/* calltype_common_definitions.tmpl will only be executed for each 'calltype' template. */}}

{{define "callGRPC"}}
func callGRPC{{.MethodName}}(ctx context.Context, node *Node, arg *{{.FQReqName}}, replyChan chan<- {{.UnexportedTypeName}}) {
	reply := new({{.FQRespName}})
	start := time.Now()
	err := grpc.Invoke(
		ctx,
		"/{{.ServPackageName}}.{{.ServName}}/{{.MethodName}}",
		arg,
		reply,
		node.conn,
	)
	s, ok := status.FromError(err)
	if ok && (s.Code() == codes.OK || s.Code() == codes.Canceled) {
		node.setLatency(time.Since(start))
	} else {
		node.setLastErr(err)
	}
	replyChan <- {{.UnexportedTypeName}}{node.id, reply, err}
}
{{end}}

{{define "trace"}}
	var ti traceInfo
	if c.mgr.opts.trace {
		ti.Trace = trace.New("gorums."+c.tstring()+".Sent", "{{.MethodName}}")
		defer ti.Finish()

		ti.firstLine.cid = c.id
		if deadline, ok := ctx.Deadline(); ok {
			ti.firstLine.deadline = time.Until(deadline)
		}
		ti.LazyLog(&ti.firstLine, false)
		ti.LazyLog(&payload{sent: true, msg: a}, false)

		defer func() {
			ti.LazyLog(&qcresult{
				ids:   resp.NodeIDs,
				reply: resp.{{.CustomRespName}},
				err:   resp.err,
			}, false)
			if resp.err != nil {
				ti.SetError()
			}
		}()
	}
{{end}}

{{define "simple_trace"}}
	var ti traceInfo
	if c.mgr.opts.trace {
		ti.Trace = trace.New("gorums."+c.tstring()+".Sent", "{{.MethodName}}")
		defer ti.Finish()

		ti.firstLine.cid = c.id
		if deadline, ok := ctx.Deadline(); ok {
			ti.firstLine.deadline = time.Until(deadline)
		}
		ti.LazyLog(&ti.firstLine, false)
		ti.LazyLog(&payload{sent: true, msg: a}, false)

		defer func() {
			ti.LazyLog(&qcresult{
				reply: resp,
				err:   err,
			}, false)
			if err != nil {
				ti.SetError()
			}
		}()
	}
{{end}}

{{define "unexported_method_signature"}}
{{- if .PerNodeArg}}
func (c *Configuration) {{.UnexportedMethodName}}(ctx context.Context, a *{{.FQReqName}}, f func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}, resp *{{.TypeName}}) {
{{- else}}
func (c *Configuration) {{.UnexportedMethodName}}(ctx context.Context, a *{{.FQReqName}}, resp *{{.TypeName}}) {
{{- end -}}
{{end}}

{{define "callLoop"}}
  expected := c.n
  replyChan := make(chan {{.UnexportedTypeName}}, expected)
  for _, n := range c.nodes {
{{- if .PerNodeArg}}
    nodeArg := f(*a, n.id)
    if nodeArg == nil {
      expected--
      continue
    }
    go callGRPC{{.MethodName}}(ctx, n, nodeArg, replyChan)
{{- else}}
    go callGRPC{{.MethodName}}(ctx, n, a, replyChan)
{{end -}}
  }
{{end}}
`

const calltype_correctable_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import (
	"context"
	"time"

	"golang.org/x/net/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

{{- end}}

{{range .Services}}

{{if .Correctable}}

/* Exported correctable method {{.MethodName}} */

{{if .PerNodeArg}}

// {{.MethodName}} asynchronously invokes a 
// correctable {{.MethodName}} quorum call on each node in configuration c,
// with the argument returned by the provided perNode
// function and returns a {{.TypeName}}, which can be used
// to inspect the quorum call reply and error when available. 
// The perNode function takes the provided arg and returns a {{.FQReqName}}
// object to be passed to the given nodeID.
// The perNode function should be thread-safe.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}, perNode func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}) *{{.TypeName}} {
	corr := &{{.TypeName}}{
		level:   LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech:  make(chan struct{}),
	}
	go c.{{.UnexportedMethodName}}(ctx, arg, perNode, corr)
	return corr
}

{{else}}

// {{.MethodName}} asynchronously invokes a
// correctable {{.MethodName}} quorum call on configuration c and returns a
// {{.TypeName}} which can be used to inspect any replies or errors
// when available.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}) *{{.TypeName}} {
	corr := &{{.TypeName}}{
		level:   LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech:  make(chan struct{}),
	}
	go c.{{.UnexportedMethodName}}(ctx, arg, corr)
	return corr
}

{{- end}}

// Get returns the reply, level and any error associated with the
// {{.MethodName}}. The method does not block until a (possibly
// itermidiate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *{{.TypeName}}) Get() (*{{.FQCustomRespName}}, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.{{.CustomRespName}}, c.level, c.err
}

// Done returns a channel that's closed when the correctable {{.MethodName}}
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or that the call returned an
// error.
func (c *{{.TypeName}}) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that's closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// disregardless of the specified level.
func (c *{{.TypeName}}) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	if level < c.level {
		close(ch)
		c.mu.Unlock()
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	c.mu.Unlock()
	return ch
}

func (c *{{.TypeName}}) set(reply *{{.FQCustomRespName}}, level int, err error, done bool) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		panic("set(...) called on a done correctable")
	}
	c.{{.CustomRespName}}, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		c.mu.Unlock()
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
	c.mu.Unlock()
}

/* Unexported correctable method {{.MethodName}} */

{{template "unexported_method_signature" . -}}
	{{- template "trace" .}}

	{{- template "callLoop" .}}

	var (
		replyValues 	= make([]*{{.FQRespName}}, 0, c.n)
		clevel      	= LevelNotSet
		reply		*{{.FQCustomRespName}}
		rlevel		int
		errs 		[]GRPCError
		quorum      	bool
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
{{- if .QFWithReq}}
			reply, rlevel, quorum = c.qspec.{{.MethodName}}QF(a, replyValues)
{{else}}
			reply, rlevel, quorum = c.qspec.{{.MethodName}}QF(replyValues)
{{end -}}
			if quorum {
				resp.set(reply, rlevel, nil, true)
				return
			}
			if rlevel > clevel {
				clevel = rlevel
				resp.set(reply, rlevel, nil, false)
			}
		case <-ctx.Done():
			resp.set(reply, clevel, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}, true)
			return
		}

		if len(errs)+len(replyValues) == expected {
			resp.set(reply, clevel, QuorumCallError{"incomplete call", len(replyValues), errs}, true)
			return
		}
	}
}

{{template "callGRPC" .}}

{{- end -}}
{{- end -}}
`

const calltype_correctable_stream_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import (
	"context"
	"io"
	"time"

	"golang.org/x/net/trace"
)
{{- end}}

{{range .Services}}

{{if .CorrectableStream}}

/* Exported correctable stream method {{.MethodName}} */

{{if .PerNodeArg}}

// {{.MethodName}} asynchronously invokes a 
// correctable {{.MethodName}} quorum call on each node in configuration c,
// with the argument returned by the provided perNode
// function and returns a {{.TypeName}}, which can be used
// to inspect the quorum call reply and error when available. 
// The perNode function takes the provided arg and returns a {{.FQReqName}}
// object to be passed to the given nodeID.
// The perNode function should be thread-safe.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}, perNode func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}) *{{.TypeName}} {
	corr := &{{.TypeName}}{
		level:  LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech: make(chan struct{}),
	}
	go c.{{.UnexportedMethodName}}(ctx, arg, perNode, corr)
	return corr
}

{{else}}

// {{.MethodName}} asynchronously invokes a correctable {{.MethodName}} quorum call
// with server side preliminary reply support on configuration c and returns a
// {{.TypeName}} which can be used to inspect any replies or errors
// when available.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}) *{{.TypeName}} {
	corr := &{{.TypeName}}{
		level:  LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech: make(chan struct{}),
	}
	go c.{{.UnexportedMethodName}}(ctx, arg, corr)
	return corr
}

{{- end}}

// Get returns the reply, level and any error associated with the
// {{.MethodName}}. The method does not block until a (possibly
// itermidiate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *{{.TypeName}}) Get() (*{{.FQCustomRespName}}, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.{{.CustomRespName}}, c.level, c.err
}

// Done returns a channel that's closed when the correctable {{.MethodName}}
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or that the call returned an
// error.
func (c *{{.TypeName}}) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that's closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// disregardless of the specified level.
func (c *{{.TypeName}}) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	if level < c.level {
		close(ch)
		c.mu.Unlock()
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	c.mu.Unlock()
	return ch
}

func (c *{{.TypeName}}) set(reply *{{.FQCustomRespName}}, level int, err error, done bool) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		panic("set(...) called on a done correctable")
	}
	c.{{.CustomRespName}}, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		c.mu.Unlock()
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
	c.mu.Unlock()
}

/* Unexported correctable stream method {{.MethodName}} */

{{template "unexported_method_signature" . -}}
	{{- template "trace" .}}

	{{- template "callLoop" .}}

	var (
		replyValues 	= make([]*{{.FQRespName}}, 0, c.n*2)
		clevel      	= LevelNotSet
		reply		*{{.FQCustomRespName}}
		rlevel      	int
		errs 		[]GRPCError
		quorum      	bool
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = appendIfNotPresent(resp.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			if c.mgr.opts.trace {
				ti.LazyLog(&payload{sent: false, id: r.nid, msg: r.reply}, false)
			}
			replyValues = append(replyValues, r.reply)
{{- if .QFWithReq}}
			reply, rlevel, quorum = c.qspec.{{.MethodName}}QF(a, replyValues)
{{else}}
			reply, rlevel, quorum = c.qspec.{{.MethodName}}QF(replyValues)
{{end -}}
			if quorum {
				resp.set(reply, rlevel, nil, true)
				return
			}
			if rlevel > clevel {
				clevel = rlevel
				resp.set(reply, rlevel, nil, false)
			}
		case <-ctx.Done():
			resp.set(reply, clevel, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}, true)
			return
		}

		if len(errs) == expected { // Can't rely on reply count.
			resp.set(reply, clevel, QuorumCallError{"incomplete call", len(replyValues), errs}, true)
			return
		}
	}
}

func callGRPC{{.MethodName}}(ctx context.Context, node *Node, arg *{{.FQReqName}}, replyChan chan<- {{.UnexportedTypeName}}) {
	x := New{{.ServName}}Client(node.conn)
	y, err := x.{{.MethodName}}(ctx, arg)
	if err != nil {
		replyChan <- {{.UnexportedTypeName}}{node.id, nil, err}
		return
	}

	for {
		reply, err := y.Recv()
		if err == io.EOF {
			return
		}
		replyChan <- {{.UnexportedTypeName}}{node.id, reply, err}
		if err != nil {
			return
		}
	}
}

{{- end -}}
{{- end -}}
`

const calltype_datatypes_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import "sync"
{{- end}}

{{range .ResponseTypes}}

{{if or .Correctable .CorrectableStream}}
// {{.TypeName}} for processing correctable {{.FQCustomRespName}} replies.
type {{.TypeName}} struct {
	mu sync.Mutex
	// the actual reply
	*{{.FQCustomRespName}}
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
{{- end}}

{{if .Future}}
// {{.TypeName}} is a future object for an asynchronous quorum call invocation.
type {{.TypeName}} struct {
	// the actual reply
	*{{.FQCustomRespName}}
	NodeIDs  []uint32
	err   error
	c     chan struct{}
}
{{- end}}

{{- end}}

{{range .InternalResponseTypes}}

{{if or .Correctable .CorrectableStream .Future .QuorumCall .StrictOrdering}}
type {{.UnexportedTypeName}} struct {
	nid   uint32
	reply *{{.FQRespName}}
	err   error
}
{{- end}}

{{- end}}
`

const calltype_future_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import (
	"context"
	"time"

	"golang.org/x/net/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)
{{end}}

{{range .Services}}

{{if .Future}}

/* Exported asynchronous quorum call method {{.MethodName}} */

{{if .PerNodeArg}}

// {{.MethodName}} asynchronously invokes a quorum call on each node in
// configuration c, with the argument returned by the provided perNode
// function and returns the result as a {{.TypeName}}, which can be used
// to inspect the quorum call reply and error when available. 
// The perNode function takes the provided arg and returns a {{.FQReqName}}
// object to be passed to the given nodeID.
// The perNode function should be thread-safe.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}, perNode func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}) *{{.TypeName}} {
	f := &{{.TypeName}}{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	go func() {
		defer close(f.c)
		c.{{.UnexportedMethodName}}(ctx, arg, perNode, f)
	}()
	return f
}

{{else}}

// {{.MethodName}} asynchronously invokes a quorum call on configuration c
// and returns a {{.TypeName}} which can be used to inspect the quorum call
// reply and error when available.
func (c *Configuration) {{.MethodName}}(ctx context.Context, arg *{{.FQReqName}}) *{{.TypeName}} {
	f := &{{.TypeName}}{
		NodeIDs: make([]uint32, 0, c.n),
		c:       make(chan struct{}, 1),
	}
	go func() {
		defer close(f.c)
		c.{{.UnexportedMethodName}}(ctx, arg, f)
	}()
	return f
}

{{- end}}

// Get returns the reply and any error associated with the {{.MethodName}}.
// The method blocks until a reply or error is available.
func (f *{{.TypeName}}) Get() (*{{.FQCustomRespName}}, error) {
	<-f.c
	return f.{{.CustomRespName}}, f.err
}

// Done reports if a reply and/or error is available for the {{.MethodName}}.
func (f *{{.TypeName}}) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

/* Unexported asynchronous quorum call method {{.MethodName}} */

{{template "unexported_method_signature" .}}
	{{- template "trace" .}}

	{{template "callLoop" .}}

	var (
		replyValues = 	make([]*{{.FQRespName}}, 0, c.n)
		reply		*{{.FQCustomRespName}}
		errs 		[]GRPCError
		quorum      	bool
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
{{- if .QFWithReq}}
			if reply, quorum = c.qspec.{{.MethodName}}QF(a, replyValues); quorum {
{{else}}
			if reply, quorum = c.qspec.{{.MethodName}}QF(replyValues); quorum {
{{end -}}
				resp.{{.CustomRespName}}, resp.err = reply, nil
				return
			}
		case <-ctx.Done():
			resp.{{.CustomRespName}}, resp.err = reply, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
			return
		}

		if len(errs)+len(replyValues) == expected {
			resp.{{.CustomRespName}}, resp.err = reply, QuorumCallError{"incomplete call", len(replyValues), errs}
			return
		}
	}
}

{{template "callGRPC" .}}

{{- end -}}
{{- end -}}
`

const calltype_multicast_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}
{{end}}

{{range .Services}}

{{if .Multicast}}

/* Exported types and methods for multicast method {{.MethodName}} */

// {{.MethodName}} is a one-way multicast call on all nodes in configuration c,
// using the same argument arg. The call is asynchronous and has no return value.
func (c *Configuration) {{.MethodName}}(arg *{{.FQReqName}}) error {
	return c.{{.UnexportedMethodName}}(arg)
}

/* Unexported types and methods for multicast method {{.MethodName}} */

func (c *Configuration) {{.UnexportedMethodName}}(arg *{{.FQReqName}}) error {
	for _, node := range c.nodes {
		go func(n *Node) {
			err := n.{{.MethodName}}Client.Send(arg)
			if err == nil {
				return
			}
			if c.mgr.logger != nil {
				c.mgr.logger.Printf("%d: {{.UnexportedMethodName}} stream send error: %v", n.id, err)
			}
		}(node)
	}

	return nil
}
{{- end -}}
{{- end -}}
`

const calltype_quorumcall_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import (
	"context"
	"time"

	"golang.org/x/net/trace"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)
{{end}}

{{range .Services}}

{{if .QuorumCall}}

/* Exported types and methods for quorum call method {{.MethodName}} */

{{if .PerNodeArg}}

// {{.MethodName}} is invoked as a quorum call on each node in configuration c,
// with the argument returned by the provided perNode function and returns the
// result. The perNode function takes a request arg and
// returns a {{.FQReqName}} object to be passed to the given nodeID.
// The perNode function should be thread-safe.
func (c *Configuration) {{.MethodName}}(ctx context.Context, a *{{.FQReqName}}, f func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}) (resp *{{.FQCustomRespName}}, err error) {
{{- else}}

// {{.MethodName}} is invoked as a quorum call on all nodes in configuration c,
// using the same argument arg, and returns the result.
func (c *Configuration) {{.MethodName}}(ctx context.Context, a *{{.FQReqName}}) (resp *{{.FQCustomRespName}}, err error) {
{{- end}}
	{{- template "simple_trace" .}}

	{{template "callLoop" .}}

	var (
		replyValues = make([]*{{.FQRespName}}, 0, expected)
		errs []GRPCError
		quorum      bool
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			if c.mgr.opts.trace {
				ti.LazyLog(&payload{sent: false, id: r.nid, msg: r.reply}, false)
			}
			replyValues = append(replyValues, r.reply)
{{- if .QFWithReq}}
			if resp, quorum = c.qspec.{{.MethodName}}QF(a, replyValues); quorum {
{{else}}
			if resp, quorum = c.qspec.{{.MethodName}}QF(replyValues); quorum {
{{end -}}
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
		}

		if len(errs)+len(replyValues) == expected {
			return resp, QuorumCallError{"incomplete call", len(replyValues), errs}
		}
	}
}

{{template "callGRPC" .}}

{{- end -}}
{{- end -}}
`

const calltype_strict_ordering_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{ $Pkg := .PackageName }}

{{if not .IgnoreImports}}
package {{ $Pkg }}

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/net/trace"
)
{{end}}

{{range .Services}}

{{if .StrictOrdering}}

{{ $stream := printf "%s_%sClient" .ServName .MethodName }}
{{ $state := printf "%sStream" .MethodName }}
/* Exported types and methods for strictly ordered quorum call method {{.MethodName}} */
type {{$state}} struct {
	mu      sync.Mutex
	nextID  uint64
	streams map[uint32]{{$stream}}
	sendQ   map[uint32]chan *{{.FQReqName}} // Maps a node ID to the send channel for that node
	recvQ   map[uint64]chan {{.UnexportedTypeName}} // Maps a message ID to the receive channel for that message
	cancel  func()
}

func (c *Configuration) New{{$state}}() (*{{$state}}, error) {
	s := &{{$state}}{
		streams: make(map[uint32]{{$stream}}),
		sendQ: make(map[uint32]chan *{{.FQReqName}}),
		recvQ: make(map[uint64]chan {{.UnexportedTypeName}}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	for _, node := range c.nodes {
		s.sendQ[node.id] = make(chan *{{.FQReqName}}, 1)
		stream, err := node.{{.MethodName}}(ctx)
		if err != nil {
			cancel()
			close(s.sendQ[node.id])
			return nil, fmt.Errorf("stream creation failed: %w", err)
		}
		s.streams[node.id] = stream

		go s.sendMsgs(node)
		go s.recvMsgs(node)
	}

	return s, nil
}

func (s *{{$state}}) sendMsgs(node *Node) {
	stream := s.streams[node.id]
	sendQ := s.sendQ[node.id]
	for msg := range sendQ {
		err := stream.SendMsg(msg)
		// TODO: figure out how to handle a stream ending prematurely
		if err != nil {
			if err != io.EOF {
				node.setLastErr(err)
			}
			return
		}
	}
}

func (s *{{$state}}) recvMsgs(node *Node) {
	stream := s.streams[node.id]
	msg := new({{.FQRespName}})
	for {
		err := stream.RecvMsg(msg)
		// TODO: figure out how to handle a stream ending prematurely
		if err != nil {
			if err != io.EOF {
				node.setLastErr(err)
			}
			return
		}
		s.mu.Lock()
		id := msg.{{.OrderingIDField}}
		if c, ok := s.recvQ[id]; ok {
			c <- {{.UnexportedTypeName}}{node.id, msg, err}
		}
		s.mu.Unlock()
	}
}

func (s *{{$state}}) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.sendQ {
		close(c)
	}
	for _, cs := range s.streams {
		// TODO: figure out if the error needs to be handled
		cs.CloseSend()
	}
}

func (s *{{$state}}) getNextID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return s.nextID
}

{{if .PerNodeArg}}

// {{.MethodName}} is invoked as a quorum call on each node in configuration c,
// with the argument returned by the provided perNode function and returns the
// result. The perNode function takes a request arg and
// returns a {{.FQReqName}} object to be passed to the given nodeID.
// The perNode function should be thread-safe.
func (c *Configuration) {{.MethodName}}(ctx context.Context, s *{{$state}}, a *{{.FQReqName}}, f func(arg {{.FQReqName}}, nodeID uint32) *{{.FQReqName}}) (resp *{{.FQCustomRespName}}, err error) {
{{- else}}

// {{.MethodName}} is invoked as a quorum call on all nodes in configuration c,
// using the same argument arg, and returns the result.
func (c *Configuration) {{.MethodName}}(ctx context.Context, s *{{$state}}, a *{{.FQReqName}}) (resp *{{.FQCustomRespName}}, err error) {
{{- end}}
	{{- template "simple_trace" .}}

	msgID := s.getNextID()
	// get the ID which will be used to return the correct responses for a request
	a.{{.OrderingIDField}} = msgID
	
	// set up a channel to collect replies
	replies := make(chan {{.UnexportedTypeName}}, c.n)
	s.mu.Lock()
	s.recvQ[msgID] = replies
	s.mu.Unlock()
	
	defer func() {
		// remove the replies channel when we are done
		s.mu.Lock()
		delete(s.recvQ, msgID)
		close(replies)
		s.mu.Unlock()
	}()
	
	// push the message to the nodes
	expected := c.n
	for _, n := range c.nodes {
{{- if .PerNodeArg}}
		nodeArg := f(*a, n.ID())
		if nodeArg == nil {
			expected--
			continue
		}
		nodeArg.{{.OrderingIDField}} = msgID
		s.sendQ[n.ID()] <- nodeArg
{{- else}}
		s.sendQ[n.ID()] <- a
{{end -}}
	}

	var (
		replyValues = make([]*{{.FQRespName}}, 0, expected)
		errs []GRPCError
		quorum      bool
	)

	for {
		select {
		case r := <-replies:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			if c.mgr.opts.trace {
				ti.LazyLog(&payload{sent: false, id: r.nid, msg: r.reply}, false)
			}
			replyValues = append(replyValues, r.reply)
{{- if .QFWithReq}}
			if resp, quorum = c.qspec.{{.MethodName}}QF(a, replyValues); quorum {
{{else}}
			if resp, quorum = c.qspec.{{.MethodName}}QF(replyValues); quorum {
{{end -}}
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{ctx.Err().Error(), len(replyValues), errs}
		}

		if len(errs)+len(replyValues) == expected {
			return resp, QuorumCallError{"incomplete call", len(replyValues), errs}
		}
	}
}

{{- end -}}
{{- end -}}
`

const node_tmpl = `
{{/* Remember to run 'make dev' after editing this file. */}}

{{- if not .IgnoreImports}}
package {{.PackageName}}

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
)
{{- end}}

// Node encapsulates the state of a node on which a remote procedure call
// can be made.
type Node struct {
	// Only assigned at creation.
	id		uint32
	addr	string
	conn	*grpc.ClientConn
	logger	*log.Logger

{{range .Clients}}
	{{.}} {{.}}
{{- end}}

{{range .Services}}
{{- if .ClientStreaming}}
	{{.MethodName}}Client {{.ServName}}_{{.MethodName}}Client
{{- end -}}
{{end}}

	mu sync.Mutex
	lastErr error
	latency time.Duration
}

func (n *Node) connect(opts managerOptions) error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, opts.grpcDialOpts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %v", err)
	}

{{range .Clients}}
	n.{{.}} = New{{.}}(n.conn)
{{- end}}

{{range .Services}}
{{if .ClientStreaming}}
  	n.{{.MethodName}}Client, err = n.{{.ServName}}Client.{{.MethodName}}(context.Background())
  	if err != nil {
  		return fmt.Errorf("stream creation failed: %v", err)
  	}
{{end}}
{{end -}}

	return nil
}

func (n *Node) close() error {
{{- range .Services -}}
{{if and .ClientStreaming .ServerStreaming}}
	_ = n.{{.MethodName}}Client.CloseSend()
{{- else if .ClientStreaming}}
	_, _ = n.{{.MethodName}}Client.CloseAndRecv()
{{- end -}}
{{end}}

	if err := n.conn.Close(); err != nil {
		if n.logger != nil {
			n.logger.Printf("%d: conn close error: %v", n.id, err)
		}
    	return fmt.Errorf("%d: conn close error: %v", n.id, err)
    }
	return nil
}
`

const qspec_tmpl = `{{/* Remember to run 'make dev' after editing this file. */}}

{{define "QFcomment"}}
// {{.MethodName}}QF is the quorum function for the {{.MethodName}}
{{- if .QuorumCall}}
// quorum call method.
{{- end -}}
{{- if .Future}}
// asynchronous quorum call method.
{{- end -}}
{{- if .Correctable}}
// correctable quorum call method.
{{- end -}}
{{- if .CorrectableStream}}
// correctable stream quourm call method.
{{- end -}}
{{- if .StrictOrdering }}
// quorum call method with strict ordering.
{{- end -}}
{{end}}

{{define "QFreply"}}
{{- if or (.QuorumCall) (.Future) (.StrictOrdering) -}}
(*{{.FQCustomRespName}}, bool)
{{end -}}
{{- if or (.Correctable) (.CorrectableStream) -}}
(*{{.FQCustomRespName}}, int, bool)
{{end -}}
{{end}}

{{define "QFmethodSignature"}}
{{- if .QFWithReq}}
{{.MethodName}}QF(req *{{.FQReqName}}, replies []*{{.FQRespName}})
{{- else}}
{{.MethodName}}QF(replies []*{{.FQRespName}})
{{- end -}}
{{end}}

{{- if not .IgnoreImports}}
package {{.PackageName}}
{{- end}}

// QuorumSpec is the interface that wraps every quorum function.
type QuorumSpec interface {
{{- range .Services}}
{{- if not .Multicast -}}
{{template "QFcomment" . -}}
{{template "QFmethodSignature" .}} {{template "QFreply" .}}
{{end}}
{{end}}
}
`

var templates = map[string]string{
	"calltype_common_definitions_tmpl": calltype_common_definitions_tmpl,
	"calltype_correctable_tmpl":        calltype_correctable_tmpl,
	"calltype_correctable_stream_tmpl": calltype_correctable_stream_tmpl,
	"calltype_datatypes_tmpl":          calltype_datatypes_tmpl,
	"calltype_future_tmpl":             calltype_future_tmpl,
	"calltype_multicast_tmpl":          calltype_multicast_tmpl,
	"calltype_quorumcall_tmpl":         calltype_quorumcall_tmpl,
	"calltype_strict_ordering_tmpl":    calltype_strict_ordering_tmpl,
	"node_tmpl":                        node_tmpl,
	"qspec_tmpl":                       qspec_tmpl,
}
