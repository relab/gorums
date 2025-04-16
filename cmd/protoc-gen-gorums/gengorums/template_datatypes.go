package gengorums

var internalOutDataType = `
{{$_ := use "gorums.EnforceVersion" .GenFile}}
{{range $intOut, $out := mapInternalOutType .GenFile .Services}}
type {{$intOut}} struct {
	nid   uint32
	reply *{{$out}}
	err   error
}
{{end}}
`

var dataTypes = internalOutDataType
