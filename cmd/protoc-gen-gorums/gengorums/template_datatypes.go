package gengorums

// This type alias is generated only once per return type for a correctable call type.
// That is, if multiple correctable calls use the same return type, this type alias
// is only generated once.
var correctableDataType = `
{{$correctable := use "gorums.Correctable" .GenFile}}
{{range $correctableOut, $customOut := mapCorrectableOutType .GenFile .Services}}
// {{$correctableOut}} is a correctable object for quorum calls returning *{{$customOut}}.
type {{$correctableOut}} = *{{$correctable}}[*{{$customOut}}]
{{end}}
`

var dataTypes = correctableDataType
