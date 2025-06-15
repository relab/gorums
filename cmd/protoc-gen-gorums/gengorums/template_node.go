package gengorums

var node = `
{{- $_ := use "gorums.EnforceVersion" .GenFile}}
{{- $callOpt := use "gorums.CallOption" .GenFile}}
{{- $node := use "gorums.Node" .GenFile}}
{{- $genFile := .GenFile}}
{{- range .Services}}
	{{- $service := .GoName}}
	{{- $nodeName := printf "%sNode" $service}}
	// {{$nodeName}} holds the node specific methods for the {{$service}} service.
	{{- reserveName $nodeName}}
	type {{$nodeName}} struct {
		node *{{$node}}
	}

	{{$funcName := printf "%sRpc" $nodeName}}
	{{- reserveName $funcName}}
	func {{$funcName}}(node *{{$node}}) {{$nodeName}} {
		return {{$nodeName}}{
			node: node,
		}
	}
{{- end}}
`
