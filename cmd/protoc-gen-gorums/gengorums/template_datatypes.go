package gengorums

// nodeIDDataType is the template for the node ID type.
var nodeIDDataType = `
{{$gorums := use "gorums.EnforceVersion" .GenFile}}
// NodeID is a type alias for the type used to identify nodes.
{{- with index .Services 0 }}
type NodeID = {{ nodeIDType . }}
{{- end }}
`

// This type alias is generated only once per return type for an async call type.
// That is, if multiple async calls use the same return type, this type alias
// is only generated once.
var asyncDataType = `
{{$async := use "gorums.Async" .GenFile}}
{{range $asyncOut, $customOut := mapAsyncOutType .GenFile .Services}}
// {{$asyncOut}} is a future for async quorum calls returning *{{$customOut}}.
type {{$asyncOut}} = *{{$async}}[NodeID, *{{$customOut}}]
{{end}}
`

// This type alias is generated only once per return type for a correctable call type.
// That is, if multiple correctable calls use the same return type, this type alias
// is only generated once.
var correctableDataType = `
{{$correctable := use "gorums.Correctable" .GenFile}}
{{range $correctableOut, $customOut := mapCorrectableOutType .GenFile .Services}}
// {{$correctableOut}} is a correctable object for quorum calls returning *{{$customOut}}.
type {{$correctableOut}} = *{{$correctable}}[NodeID, *{{$customOut}}]
{{end}}
`

var dataTypes = nodeIDDataType + asyncDataType + correctableDataType
