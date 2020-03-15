package internalgorums

var correctableCallVariables = `
{{$context := use "context.Context" .GenFile}}
{{$opts := use "grpc.CallOption" .GenFile}}
{{$correctableOut := printf "Correctable%s" $out}}
{{$appendFn := printf "append"}}
{{if correctableStream .Method}}
{{$appendFn = printf "appendIfNotPresent"}}
{{$correctableOut = printf "CorrectableStream%s" $out}}
{{end}}
`

var correctableCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
{{if hasPerNodeArg .Method}}
// {{$method}} asynchronously invokes a correctable quorum call on each node
// in configuration c, with the argument returned by the provided function f
// and returns a {{$correctableOut}}, which can be used to inspect
// the reply and error when available.
// The provide per node function f takes the provided {{$in}} argument
// and returns an {{$out}} object to be passed to the given nodeID.
// The per node function f should be thread-safe.
{{else}}
// {{$method}} asynchronously invokes a correctable quorum call on each node
// in configuration c and returns a {{$correctableOut}}, which can be used
// to inspect any replies or errors when available.
{{if correctableStream .Method -}}
// This method supports server-side preliminary replies (correctable stream).
{{end -}}
{{end -}}
{{end -}}
`

var correctableCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, opts ...{{$opts}}) ` +
	`*{{$correctableOut}} {`

var correctableCallBody = `
	corr := &{{$correctableOut}}{
		level:   LevelNotSet,
		NodeIDs: make([]uint32, 0, c.n),
		donech:  make(chan struct{}),
	}
	go c.{{unexport .Method.GoName}}(ctx, in{{perNodeArg .Method ", f"}}, corr, opts...)
	return corr
}
`

var correctableCallInterface = `
// Get returns the reply, level and any error associated with the
// {{$method}}. The method does not block until a (possibly
// itermidiate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *{{$correctableOut}}) Get() (*{{$customOut}}, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.{{$customOut}}, c.level, c.err
}

// Done returns a channel that will be closed when the correctable {{$method}}
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *{{$correctableOut}}) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *{{$correctableOut}}) Watch(level int) <-chan struct{} {
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

func (c *{{$correctableOut}}) set(reply *{{$customOut}}, level int, err error, done bool) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		panic("set(...) called on a done correctable")
	}
	c.{{$customOut}}, c.level, c.err, c.done = reply, level, err, done
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
`

var correctableCallUnexportedSignature = `
func (c *Configuration) {{unexport .Method.GoName}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, resp *{{$correctableOut}}, opts ...{{$opts}}) {
`

var correctableCallReply = `
	var (
		{{- if correctableStream .Method}}
		//TODO(meling) don't recall why we need n*2 reply slots?
		replyValues	= make([]*{{$out}}, 0, c.n*2)
		{{else}}
		replyValues	= make([]*{{$out}}, 0, c.n)
		{{end -}}
		clevel		= LevelNotSet
		reply		*{{$customOut}}
		rlevel		int
		errs		[]GRPCError
		quorum		bool
	)

	for {
		select {
		case r := <-replyChan:
			resp.NodeIDs = {{$appendFn}}(resp.NodeIDs, r.nid)
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{template "traceLazyLog"}}
			replyValues = append(replyValues, r.reply)
			reply, rlevel, quorum = c.qspec.{{$method}}QF({{withQFArg .Method "in, "}}replyValues)
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
		{{- if correctableStream .Method}}
		if len(errs) == expected { // Can't rely on reply count.
		{{else}}
		if len(errs)+len(replyValues) == expected {
		{{end -}}
			resp.set(reply, clevel, QuorumCallError{"incomplete call", len(replyValues), errs}, true)
			return
		}
	}
}
`

var correctableStreamCallGrpc = `
func (n *Node) {{$method}}(ctx {{$context}}, in *{{$in}}, replyChan chan<- {{$intOut}}) {
	x := New{{serviceName .Method}}Client(n.conn)
	y, err := x.{{$method}}(ctx, in)
	if err != nil {
		replyChan <- {{$intOut}}{n.id, nil, err}
		return
	}

	for {
		reply, err := y.Recv()
		if err == {{use "io.EOF" .GenFile}} {
			return
		}
		replyChan <- {{$intOut}}{n.id, reply, err}
		if err != nil {
			return
		}
	}
}
`

var correctableCall = commonVariables +
	correctableCallVariables +
	correctableCallComment +
	correctableCallSignature +
	correctableCallBody +
	correctableCallInterface +
	correctableCallUnexportedSignature +
	quorumCallLoop +
	correctableCallReply +
	nodeCallGrpc

var correctableStreamCall = commonVariables +
	correctableCallVariables +
	correctableCallComment +
	correctableCallSignature +
	correctableCallBody +
	correctableCallInterface +
	correctableCallUnexportedSignature +
	quorumCallLoop +
	correctableCallReply +
	correctableStreamCallGrpc
