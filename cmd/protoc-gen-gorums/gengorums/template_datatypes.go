package gengorums

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
// for a async call type. That is, if multiple async calls use the same
// return type, this struct and associated methods are only generated once.
var asyncDataType = `
{{$async := use "gorums.Async" .GenFile}}
{{range $asyncOut, $customOut := mapAsyncOutType .GenFile .Services}}
{{$customOutField := field $customOut}}
// {{$asyncOut}} is a async object for processing replies.
type {{$asyncOut}} struct {
	*{{$async}}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *{{$asyncOut}}) Get() (*{{$customOut}}, error) {
	resp, err := f.Async.Get()
	if err != nil {
		return nil, err
	}
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
	if err != nil {
		return nil, level, err
	}
	return resp.(*{{$customOut}}), level, err
}
{{- end -}}
`

var datatypes = internalOutDataType +
	asyncDataType +
	correctableDataType
