package gengorums

// TODO(meling) consider to replace this check with the hasAPIType func;
// hash map keys must be prefixed with calltype or filename.
var multicastRefImports = `
{{if contains $out "."}}
// Reference imports to suppress errors if they are not otherwise used.
var _ {{$out}}
{{end}}
`

// TODO(meling) multicast does not support per_node_arg yet.
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
	data, err := marshaler.Marshal(in)
	if err != nil {
		return {{$errorf}}("failed to marshal message: %w", err)
	}
	msg := &{{$gorumsMsg}}{
		ID: msgID,
		MethodID: {{$unexportMethod}}MethodID,
		Data: data,
	}
{{end -}}
	for _, n := range c.nodes {
{{- if hasPerNodeArg .Method}}
		nodeArg := f(in, n.ID())
		if nodeArg == nil {
			continue
		}
		data, err := marshaler.Marshal(nodeArg)
		if err != nil {
			return {{$errorf}}("failed to marshal message: %w", err)
		}
		msg := &{{$gorumsMsg}}{
			ID: msgID,
			MethodID: {{$unexportMethod}}MethodID,
			Data: data,
		}
{{- end}}
		n.sendQ <- msg
	}
	return nil
}
`

var multicastCall = commonVariables + orderingVariables + multicastRefImports + multicastMethod
