package gengorums

var unicastVar = rpcVar + `{{$callOpt := use "gorums.CallOption" .GenFile}}`

var unicastSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, opts ...{{$callOpt}}) {
`

var unicastBody = `	cd := {{$callData}}{
		Message:  in,
		Method: "{{$fullName}}",
	}

	n.Node.Unicast(ctx, cd, opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	quorumCallComment +
	unicastSignature +
	unicastBody
