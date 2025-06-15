package gengorums

var unicastVar = rpcVar + `
{{- $callOpt := use "gorums.CallOption" .GenFile}}
{{- $node := use "gorums.Node" .GenFile}}
{{- $nodeName := printf "%sNode" .Method.Parent.GoName}}
{{- $node := use "gorums.Node" .GenFile}}
`

var unicastCallComment = `
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} is a one-way call; no replies are processed.
{{end -}}
`

var unicastSignature = `func (n {{$nodeName}}) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, opts ...{{$callOpt}}) {
`

var unicastBody = `	cd := {{$callData}}{
		Message:  in,
		Method: "{{$fullName}}",
	}

	n.node.Unicast(ctx, cd, opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	unicastCallComment +
	unicastSignature +
	unicastBody
