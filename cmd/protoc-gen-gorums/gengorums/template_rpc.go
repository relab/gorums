package gengorums

var remoteCallComment = `
{{- if .Decl}}
{{$comments := .Method.Comments.Leading}}
{{if ne $comments ""}}
{{$comments -}}
{{else}}
// {{$method}} executes a remote call on the client's node and returns its
// reply.
//
// Example:
//
//	resp, err := nodeClient.{{$method}}(ctx, in)
{{end -}}
{{end -}}
`

var remoteCallVar = `
{{$genFile := .GenFile}}
{{$rpc := use "gorumsimpl.RemoteCall" .GenFile}}
{{$impl := printf "%sNodeClient" (unexport $service)}}
{{$local := localNames "c ctx in" $in $out $context $rpc}}
`

var remoteCallSignature = `{{if not .Decl}}func ({{$local.c}} {{$impl}}) {{end}}{{$method}}(` +
	`{{$local.ctx}} {{$context}}, {{$local.in}} *{{$in}})` +
	` (*{{$out}}, error)`

var remoteCallBody = `{{if .Decl}}
{{else}} {
	return {{$rpc}}[*{{$in}}, *{{$out}}]({{$local.ctx}}, {{$local.c}}.node, {{$local.in}}, "{{$fullName}}")
}
{{end}}`

var remoteCall = commonVariables +
	remoteCallVar +
	remoteCallComment +
	remoteCallSignature +
	remoteCallBody
