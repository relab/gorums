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

var multicastCall = commonVariables + orderingVariables + multicastRefImports + multicastMethod
