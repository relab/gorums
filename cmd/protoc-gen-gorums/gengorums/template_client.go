package gengorums

// gorums need to be imported in the zorums file
var clientVariables = `
{{$context := use "context.Context" .GenFile}}
{{$_ := use "gorums.EnforceVersion" .GenFile}}
{{$context := use "context.Context" .GenFile}}
{{$callOpt := use "gorums.CallOption" .GenFile}}
`

var clientConfigurationInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the client-side Configuration API for the {{$service}} Service
type {{$service}}ConfigurationClient interface {
	{{- range .Methods}}
		{{- if isConfigurationCall .}}
			{{- if isOneway .}}
				{{- $customOut := customOut $genFile .}}
				{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, opts ...{{$callOpt}})
			{{- else if or (isCorrectable .) (isAsync .)}}
				{{- $customOut := outType . (customOut $genFile .)}}
				{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) *{{$customOut}}
			{{- else}}
				{{- $customOut := customOut $genFile .}}
				{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) (resp *{{$customOut}}, err error)
			{{- end}}
		{{- end}}
	{{- end}}
}
// enforce interface compliance
var _ {{$service}}ConfigurationClient = (*Configuration)(nil)
{{- end}}
`

var clientNodeInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the client-side Node API for the {{$service}} Service
type {{$service}}NodeClient interface {
	{{- range .Methods}}
		{{- if not (isConfigurationCall .)}}
			{{- if isOneway .}}
				{{- $customOut := customOut $genFile .}}
				{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}, opts ...{{$callOpt}})
			{{- else}}
				{{- $customOut := customOut $genFile .}}
				{{.GoName}}(ctx {{$context}}, in *{{in $genFile .}} {{perNodeFnType $genFile . ", f"}}) (resp *{{$customOut}}, err error)
			{{- end}}
		{{- end}}
	{{- end}}
}
// enforce interface compliance
var _ {{$service}}NodeClient = (*Node)(nil)
{{- end}}
`

var client = clientVariables + clientConfigurationInterface + clientNodeInterface
