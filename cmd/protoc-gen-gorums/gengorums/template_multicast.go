package gengorums

var multicastRefImports = `
{{if contains $out "."}}
// Reference imports to suppress errors if they are not otherwise used.
var _ {{$out}}
{{end}}
`

var multicastMethod = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a one-way multicast call on all nodes in configuration c,
// with the same in argument. The call is asynchronous and has no return value.
{{end -}}
func (c *Configuration) {{$method}}(in *{{$in}}{{perNodeFnType .GenFile .Method ", f"}}) error {
	msgID := c.mgr.nextMsgID()
{{if not (hasPerNodeArg .Method) -}}
	metadata := &{{$gorumsMD}}{
		MessageID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
{{end -}}
	for _, n := range c.nodes {
{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.ID())
		if nodeArg == nil {
			continue
		}
		metadata := &{{$gorumsMD}}{
			MessageID: msgID,
			MethodID: {{$unexportMethod}}MethodID,
		}
		msg := &gorumsMessage{metadata: metadata, message: nodeArg}
{{- end}}
		n.sendQ <- msg
	}
	return nil
}
`

var multicastSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}}) {
`

var multicastBody = `
	cd := {{$callData}}{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: {{$unexportMethod}}MethodID,
	}
{{- if hasPerNodeArg .Method}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	{{use "gorums.Multicast" $genFile}}(ctx, cd)
}
`

var multicastCall = commonVariables +
	qcVar +
	quorumCallComment +
	multicastSignature +
	multicastBody
