package gengorums

// gorums need to be imported in the zorums file
var nodeVariables = `
{{- $_ := use "gorums.EnforceVersion" .GenFile}}
{{- $callOpt := use "gorums.CallOption" .GenFile}}
{{- $node := use "gorums.Node" .GenFile}}
`

var nodeStructs = `
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $nodeName := printf "%sNode" $service}}
	// {{$nodeName}} holds the node specific methods for the {{$service}} service.
	{{- reserveName $nodeName}}
	type {{$nodeName}} struct {
		*{{$node}}
	}
{{- end}}
`

var node = nodeVariables + nodeStructs
