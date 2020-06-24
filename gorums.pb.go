// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.24.0
// 	protoc        v3.12.3
// source: gorums.proto

package gorums

import (
	proto "github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

var file_gorums_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50000,
		Name:          "gorums.quorumcall",
		Tag:           "varint,50000,opt,name=quorumcall",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50001,
		Name:          "gorums.async",
		Tag:           "varint,50001,opt,name=async",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50002,
		Name:          "gorums.ordered",
		Tag:           "varint,50002,opt,name=ordered",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50003,
		Name:          "gorums.multicast",
		Tag:           "varint,50003,opt,name=multicast",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50004,
		Name:          "gorums.correctable",
		Tag:           "varint,50004,opt,name=correctable",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50005,
		Name:          "gorums.unicast",
		Tag:           "varint,50005,opt,name=unicast",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50006,
		Name:          "gorums.concurrent",
		Tag:           "varint,50006,opt,name=concurrent",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50007,
		Name:          "gorums.async_handler",
		Tag:           "varint,50007,opt,name=async_handler",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50010,
		Name:          "gorums.per_node_arg",
		Tag:           "varint,50010,opt,name=per_node_arg",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptor.MethodOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         51000,
		Name:          "gorums.custom_return_type",
		Tag:           "bytes,51000,opt,name=custom_return_type",
		Filename:      "gorums.proto",
	},
}

// Extension fields to descriptor.MethodOptions.
var (
	// optional bool quorumcall = 50000;
	E_Quorumcall = &file_gorums_proto_extTypes[0]
	// optional bool async = 50001;
	E_Async = &file_gorums_proto_extTypes[1]
	// optional bool ordered = 50002;
	E_Ordered = &file_gorums_proto_extTypes[2]
	// optional bool multicast = 50003;
	E_Multicast = &file_gorums_proto_extTypes[3]
	// optional bool correctable = 50004;
	E_Correctable = &file_gorums_proto_extTypes[4]
	// optional bool unicast = 50005;
	E_Unicast = &file_gorums_proto_extTypes[5]
	// optional bool concurrent = 50006;
	E_Concurrent = &file_gorums_proto_extTypes[6]
	// optional bool async_handler = 50007;
	E_AsyncHandler = &file_gorums_proto_extTypes[7]
	// optional bool per_node_arg = 50010;
	E_PerNodeArg = &file_gorums_proto_extTypes[8]
	// optional string custom_return_type = 51000;
	E_CustomReturnType = &file_gorums_proto_extTypes[9]
)

var File_gorums_proto protoreflect.FileDescriptor

var file_gorums_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06,
	0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
	0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x3a, 0x40, 0x0a, 0x0a, 0x71, 0x75, 0x6f, 0x72,
	0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd0, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a,
	0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x3a, 0x36, 0x0a, 0x05, 0x61, 0x73,
	0x79, 0x6e, 0x63, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x18, 0xd1, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x61, 0x73, 0x79,
	0x6e, 0x63, 0x3a, 0x3a, 0x0a, 0x07, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x65, 0x64, 0x12, 0x1e, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd2, 0x86,
	0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x65, 0x64, 0x3a, 0x3e,
	0x0a, 0x09, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x1e, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65,
	0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd3, 0x86, 0x03, 0x20,
	0x01, 0x28, 0x08, 0x52, 0x09, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x3a, 0x42,
	0x0a, 0x0b, 0x63, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x12, 0x1e, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd4, 0x86,
	0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0b, 0x63, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62,
	0x6c, 0x65, 0x3a, 0x3a, 0x0a, 0x07, 0x75, 0x6e, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x1e, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd5, 0x86,
	0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x75, 0x6e, 0x69, 0x63, 0x61, 0x73, 0x74, 0x3a, 0x40,
	0x0a, 0x0a, 0x63, 0x6f, 0x6e, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x74, 0x12, 0x1e, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d,
	0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd6, 0x86, 0x03,
	0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x63, 0x6f, 0x6e, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x74,
	0x3a, 0x45, 0x0a, 0x0d, 0x61, 0x73, 0x79, 0x6e, 0x63, 0x5f, 0x68, 0x61, 0x6e, 0x64, 0x6c, 0x65,
	0x72, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x73, 0x18, 0xd7, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0c, 0x61, 0x73, 0x79, 0x6e, 0x63,
	0x48, 0x61, 0x6e, 0x64, 0x6c, 0x65, 0x72, 0x3a, 0x42, 0x0a, 0x0c, 0x70, 0x65, 0x72, 0x5f, 0x6e,
	0x6f, 0x64, 0x65, 0x5f, 0x61, 0x72, 0x67, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64,
	0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xda, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x0a, 0x70, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x3a, 0x4e, 0x0a, 0x12, 0x63,
	0x75, 0x73, 0x74, 0x6f, 0x6d, 0x5f, 0x72, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x5f, 0x74, 0x79, 0x70,
	0x65, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e,
	0x73, 0x18, 0xb8, 0x8e, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x63, 0x75, 0x73, 0x74, 0x6f,
	0x6d, 0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x42, 0x19, 0x5a, 0x17, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f,
	0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_gorums_proto_goTypes = []interface{}{
	(*descriptor.MethodOptions)(nil), // 0: google.protobuf.MethodOptions
}
var file_gorums_proto_depIdxs = []int32{
	0,  // 0: gorums.quorumcall:extendee -> google.protobuf.MethodOptions
	0,  // 1: gorums.async:extendee -> google.protobuf.MethodOptions
	0,  // 2: gorums.ordered:extendee -> google.protobuf.MethodOptions
	0,  // 3: gorums.multicast:extendee -> google.protobuf.MethodOptions
	0,  // 4: gorums.correctable:extendee -> google.protobuf.MethodOptions
	0,  // 5: gorums.unicast:extendee -> google.protobuf.MethodOptions
	0,  // 6: gorums.concurrent:extendee -> google.protobuf.MethodOptions
	0,  // 7: gorums.async_handler:extendee -> google.protobuf.MethodOptions
	0,  // 8: gorums.per_node_arg:extendee -> google.protobuf.MethodOptions
	0,  // 9: gorums.custom_return_type:extendee -> google.protobuf.MethodOptions
	10, // [10:10] is the sub-list for method output_type
	10, // [10:10] is the sub-list for method input_type
	10, // [10:10] is the sub-list for extension type_name
	0,  // [0:10] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() { file_gorums_proto_init() }
func file_gorums_proto_init() {
	if File_gorums_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_gorums_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   0,
			NumExtensions: 10,
			NumServices:   0,
		},
		GoTypes:           file_gorums_proto_goTypes,
		DependencyIndexes: file_gorums_proto_depIdxs,
		ExtensionInfos:    file_gorums_proto_extTypes,
	}.Build()
	File_gorums_proto = out.File
	file_gorums_proto_rawDesc = nil
	file_gorums_proto_goTypes = nil
	file_gorums_proto_depIdxs = nil
}
