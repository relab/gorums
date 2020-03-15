package internalgorums

// Common variables used in several template functions.
var commonVariables = `
{{$method := .Method.GoName}}
{{$in := in .GenFile .Method}}
{{$out := out .GenFile .Method}}
{{$customOut := customOut .GenFile .Method}}
{{$intOut := printf "internal%s" $out}}
{{$unexportOutput := unexport .Method.Output.GoIdent.GoName}}
`

var quorumCallVariables = `
{{quorumCallImports .GenFile}}
{{$context := context .GenFile}}
{{$opts := opts .GenFile .Method}}
`

var quorumCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
{{if hasPerNodeArg .Method}}
// {{$method}} is a quorum call invoked on each node in configuration c,
// with the argument returned by the provided function f, and returns the combined result.
// The per node function f receives a copy of the {{$in}} request argument and
// returns a {{$in}} manipulated to be passed to the given nodeID.
// The function f must be thread-safe.
{{else}}
// {{$method}} is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
{{end -}}
{{end -}}
`

var quorumCallSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}` +
	`, opts ...{{$opts}})` +
	`(resp *{{$customOut}}, err error) {
`

var quorumCallLoop = `
	{{- template "trace" .}}
	expected := c.n
	replyChan := make(chan {{$intOut}}, expected)
	for _, n := range c.nodes {
		{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.id)
		if nodeArg == nil {
			expected--
			continue
		}
		go callGRPC{{$method}}(ctx, n, nodeArg, replyChan)
		{{else}}
		go callGRPC{{$method}}(ctx, n, in, replyChan)
		{{end -}}
	}
`

var quorumCallReply = `
	var (
		replyValues = make([]*{{$out}}, 0, expected)
		errs        []GRPCError
		quorum      bool
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			{{template "traceLazyLog"}}
			replyValues = append(replyValues, r.reply)
			if resp, quorum = c.qspec.{{$method}}QF({{withQFArg .Method "in, "}}replyValues); quorum {
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
`

//TODO(meling) we could consider to make this a method on Node
var callGrpc = `
func callGRPC{{$method}}(ctx {{$context}}, node *Node, in *{{$in}}, replyChan chan<- {{$intOut}}) {
	reply := new({{$out}})
	start := time.Now()
	err := node.conn.Invoke(ctx, "{{fullName .Method}}", in, reply)
	s, ok := status.FromError(err)
	if ok && (s.Code() == codes.OK || s.Code() == codes.Canceled) {
		node.setLatency(time.Since(start))
	} else {
		node.setLastErr(err)
	}
	replyChan <- {{$intOut}}{node.id, reply, err}
}
`

var quorumCall = commonVariables +
	quorumCallVariables +
	quorumCallComment +
	quorumCallSignature +
	quorumCallLoop +
	quorumCallReply +
	callGrpc