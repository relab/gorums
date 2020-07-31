package gengorums

var orderingIDs = `
{{$methods := methods .Services}}
{{range $index, $method := $methods}}
const {{unexport $method.GoName}}MethodID int32 = {{$index}}
{{- end}}
`

var orderingMethods = `
var orderingMethods = map[int32]{{use "gorums.MethodInfo" .GenFile}}{
	{{$genFile := .GenFile}}
	{{$methods := methods .Services}}
	{{range $index, $method := $methods}}
		{{$index}}: { RequestType: new({{in $genFile $method}}).ProtoReflect(), ResponseType: new({{out $genFile $method}}).ProtoReflect() },
	{{- end}}
}
`

var internalOutDataType = `
{{range $intOut, $out := mapInternalOutType .GenFile .Services}}
type {{$intOut}} struct {
	nid   uint32
	reply *{{$out}}
	err   error
}
{{end}}
`

// This struct and API functions are generated only once per return type
// for a future call type. That is, if multiple future calls use the same
// return type, this struct and associated methods are only generated once.
var futureDataType = `
{{$future := use "gorums.Future" .GenFile}}
{{range $futureOut, $customOut := mapFutureOutType .GenFile .Services}}
{{$customOutField := field $customOut}}
// {{$futureOut}} is a future object for processing replies.
type {{$futureOut}} struct {
	*{{$future}}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *{{$futureOut}}) Get() (*{{$customOut}}, error) {
	resp, err := f.Future.Get()
	return resp.(*{{$customOut}}), err
}
{{end}}
`

// This struct and API functions are generated only once per return type
// for a correctable call type. That is, if multiple correctable calls use the same
// return type, this struct and associated methods are only generated once.
var correctableDataType = `
{{$correctable := use "gorums.Correctable" .GenFile}}
{{range $correctableOut, $customOut := mapCorrectableOutType .GenFile .Services}}
{{$customOutField := field $customOut}}
// {{$correctableOut}} is a correctable object for processing replies.
type {{$correctableOut}} struct {
	*{{$correctable}}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *{{$correctableOut}}) Get() (*{{$customOut}}, int, error) {
	resp, level, err := c.Correctable.Get()
	return resp.(*{{$customOut}}), level, err
}
{{- end -}}
`

var datatypes = orderingIDs +
	orderingMethods +
	internalOutDataType +
	futureDataType +
	correctableDataType
