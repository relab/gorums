package main

import (
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	"github.com/gogo/protobuf/vanity"
	"github.com/gogo/protobuf/vanity/command"

	_ "github.com/relab/gorums/plugins/gorums"
)

func main() {
	req := command.Read()
	files := req.GetProtoFile()
	files = vanity.FilterFiles(files, vanity.NotGoogleProtobufDescriptorProto)

	vanity.ForEachFieldInFilesExcludingExtensions(vanity.OnlyProto2(files), vanity.TurnOffNullableForNativeTypesWithoutDefaultsOnly)

	for _, opt := range []func(*descriptor.FileDescriptorProto){
		vanity.TurnOnMarshalerAll,
		vanity.TurnOnSizerAll,
		vanity.TurnOnUnmarshalerAll,
		vanity.TurnOnStringerAll,
		vanity.TurnOnEnumStringerAll,

		vanity.TurnOffGoUnrecognizedAll,
		vanity.TurnOffGoEnumPrefixAll,
		vanity.TurnOffGoEnumStringerAll,
		vanity.TurnOffGoStringerAll,
	} {
		vanity.ForEachFile(files, opt)
	}

	resp := command.Generate(req)
	command.Write(resp)
}
