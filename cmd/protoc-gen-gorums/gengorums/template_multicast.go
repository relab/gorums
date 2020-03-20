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
func (c *Configuration) {{$method}}(in *{{$in}}) error {
	for _, node := range c.nodes {
		go func(n *Node) {
			err := n.{{unexport $method}}Client.Send(in)
			if err == nil {
				return
			}
			if c.mgr.logger != nil {
				c.mgr.logger.Printf("%d: {{$method}} stream send error: %v", n.id, err)
			}
		}(node)
	}
	return nil
}
`

var multicastCall = commonVariables + multicastRefImports + multicastMethod
