package gengorums

// gorums need to be imported in the zorums file
var clientVariables = `
{{$context := use "context.Context" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
`

var clientInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the client-side API for the {{$service}} Service
type {{$service}}Client interface {
	{{- range .Methods}}
	{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}}) (resp *{{out $genFile .}}, err error)
	{{- end}}
}
{{- end}}
`

var client = clientVariables + clientInterface
