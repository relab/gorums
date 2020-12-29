package gengorums

var multicastRefImports = `
{{if contains $out "."}}
// Reference imports to suppress errors if they are not otherwise used.
var _ {{$out}}
{{end}}
`

var mcVar = `
{{$callData := use "gorums.QuorumCallData" .GenFile}}
{{$genFile := .GenFile}}
{{$unexportMethod := unexport .Method.GoName}}
{{$context := use "context.Context" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var multicastSignature = `func (c *Configuration) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}` +
	`{{perNodeFnType .GenFile .Method ", f"}},` +
	`opts ...{{$callOpt}}) {
`

var multicastBody = `
	cd := {{$callData}}{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		Method: "{{$fullName}}",
	}
{{- if hasPerNodeArg .Method}}
{{$protoMessage := use "protoreflect.ProtoMessage" .GenFile}}
	cd.PerNodeArgFn = func(req {{$protoMessage}}, nid uint32) {{$protoMessage}} {
		return f(req.(*{{$in}}), nid)
	}
{{- end}}

	{{use "gorums.Multicast" $genFile}}(ctx, cd, opts...)
}
`

var multicastCall = commonVariables +
	mcVar +
	multicastRefImports +
	quorumCallComment +
	multicastSignature +
	multicastBody
