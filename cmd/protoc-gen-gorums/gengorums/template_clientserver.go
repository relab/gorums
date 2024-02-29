package gengorums

var clientServerVariables = `
{{$callData := use "gorums.CallData" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$grpc := use "grpc.GRPC" .GenFile}}
`

var clientServerInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// clientServer is the client server API for the {{$service}} Service
type clientServer interface {
	{{- range .Methods}}
	{{- if isBroadcastCall .}}
	client{{.GoName}}(ctx {{$context}}, request *{{out $genFile .}}) (*{{out $genFile .}}, error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var clientServerDesc = `
{{$genFile := .GenFile}}
var clientServer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "protos.ClientServer",
	HandlerType: (*clientServer)(nil),
	Methods: []grpc.MethodDesc{
		{{range .Services -}}
		{{- range .Methods}}
		{{- if isBroadcastCall .}}
		{
		MethodName: "Client{{.GoName}}",
		Handler:    _client{{.GoName}},
		},
		{{- end}}
		{{- end}}
		{{- end}}
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "",
}
`

var clientServer = clientServerVariables + clientServerInterface + clientServerDesc
