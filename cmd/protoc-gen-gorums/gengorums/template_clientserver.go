package gengorums

var clientServerVariables = `
{{$callData := use "gorums.CallData" .GenFile}}
`

var clientServerHandlers = `
{{$genFile := .GenFile}}
func registerClientServerHandlers(srv *clientServerImpl) {
	{{range .Services -}}
	{{- range .Methods}}
	{{- if isBroadcastCall .}}
	srv.RegisterHandler("{{.Desc.FullName}}", gorums.ClientHandler(srv.client{{.GoName}}))
	{{- end}}
	{{- end}}
	{{- end}}
}
`

var clientServer = clientServerVariables + clientServerHandlers
