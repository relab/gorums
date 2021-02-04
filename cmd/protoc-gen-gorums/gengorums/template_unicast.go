package gengorums

var unicastVar = rpcVar + `{{$callOpt := use "gorums.CallOption" .GenFile}}`

var unicastSignature = `func (n *Node) {{$method}}(` +
	`ctx {{$context}}, in *{{$in}}, opts ...{{$callOpt}}) {
`

var unicastBody = `
	cd := {{$callData}}{
		Node:     n.Node,
		Message:  in,
		Method: "{{$fullName}}",
	}

	{{use "gorums.Unicast" $genFile}}(ctx, cd, opts...)
}
`

var unicastCall = commonVariables +
	unicastVar +
	multicastRefImports +
	quorumCallComment +
	unicastSignature +
	unicastBody
