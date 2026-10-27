package gengorums

var serverVariables = `
{{$context := use "gorums.ServerContext" .GenFile}}
`

var serverInterface = `
{{$genFile := .GenFile}}
{{range .Services -}}
{{$service := .GoName}}
// {{$service}} is the server-side API for the {{$service}} Service
type {{$service}}Server interface {
	{{- range .Methods}}
	{{- if isOneway .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}})
	{{- else if isStreamingServer .}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}, func(*{{out $genFile .}}))
	{{- else}}
	{{.GoName}}({{$context}}, *{{in $genFile .}}) (*{{out $genFile .}}, error)
	{{- end}}
	{{- end}}
}
{{- end}}
`

var registerInterface = `
{{$genFile := .GenFile}}
{{$gorumsMessage := use "gorums.Message" .GenFile}}
{{$asProto := use "gorums.AsProto" .GenFile}}
{{$newMessage := use "gorums.NewResponseMessage" $genFile}}
{{$gorumsServer := use "gorums.Server" $genFile}}
{{range .Services -}}
{{$service := .GoName}}
{{$reg := localNames "srv impl" $context $gorumsMessage $asProto $newMessage (methodTypes $genFile .)}}
func Register{{$service}}Server({{$reg.srv}} *{{$gorumsServer}}, {{$reg.impl}} {{$service}}Server) {
	{{- range .Methods}}
	{{- $local := localNames "ctx in req resp err out" $context $gorumsMessage $asProto $newMessage (in $genFile .) (out $genFile .) $reg.impl}}
	{{$reg.srv}}.RegisterHandler("{{.Desc.FullName}}", func({{$local.ctx}} {{$context}}, {{$local.in}} *{{$gorumsMessage}}) (*{{$gorumsMessage}}, error) {
		{{$local.req}} := {{$asProto}}[*{{in $genFile .}}]({{$local.in}})
		{{- if isOneway .}}
		{{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}})
		return nil, nil
		{{- else if isStreamingServer .}}
		{{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}}, func({{$local.resp}} *{{out $genFile .}}) {
			{{$local.out}} := {{$newMessage}}({{$local.in}}, {{$local.resp}})
			{{$local.ctx}}.SendMessage({{$local.out}})
		})
		return nil, nil
		{{- else }}
		{{$local.resp}}, {{$local.err}} := {{$reg.impl}}.{{.GoName}}({{$local.ctx}}, {{$local.req}})
		if {{$local.err}} != nil {
			return nil, {{$local.err}}
		}
		return {{$newMessage}}({{$local.in}}, {{$local.resp}}), nil
		{{- end}}
	})
	{{- end}}
}
{{- end}}
`

var server = serverVariables + serverInterface + registerInterface
