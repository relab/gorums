// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.3
// 	protoc        v5.29.2
// source: gorums.proto

package gorums

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

var file_gorums_proto_extTypes = []protoimpl.ExtensionInfo{
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50001,
		Name:          "gorums.rpc",
		Tag:           "varint,50001,opt,name=rpc",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50002,
		Name:          "gorums.unicast",
		Tag:           "varint,50002,opt,name=unicast",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50003,
		Name:          "gorums.multicast",
		Tag:           "varint,50003,opt,name=multicast",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50004,
		Name:          "gorums.quorumcall",
		Tag:           "varint,50004,opt,name=quorumcall",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50005,
		Name:          "gorums.correctable",
		Tag:           "varint,50005,opt,name=correctable",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50010,
		Name:          "gorums.async",
		Tag:           "varint,50010,opt,name=async",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*bool)(nil),
		Field:         50020,
		Name:          "gorums.per_node_arg",
		Tag:           "varint,50020,opt,name=per_node_arg",
		Filename:      "gorums.proto",
	},
	{
		ExtendedType:  (*descriptorpb.MethodOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         50030,
		Name:          "gorums.custom_return_type",
		Tag:           "bytes,50030,opt,name=custom_return_type",
		Filename:      "gorums.proto",
	},
}

// Extension fields to descriptorpb.MethodOptions.
var (
	// call types
	//
	// optional bool rpc = 50001;
	E_Rpc = &file_gorums_proto_extTypes[0]
	// optional bool unicast = 50002;
	E_Unicast = &file_gorums_proto_extTypes[1]
	// optional bool multicast = 50003;
	E_Multicast = &file_gorums_proto_extTypes[2]
	// optional bool quorumcall = 50004;
	E_Quorumcall = &file_gorums_proto_extTypes[3]
	// optional bool correctable = 50005;
	E_Correctable = &file_gorums_proto_extTypes[4]
	// options for call types
	//
	// optional bool async = 50010;
	E_Async = &file_gorums_proto_extTypes[5]
	// optional bool per_node_arg = 50020;
	E_PerNodeArg = &file_gorums_proto_extTypes[6]
	// optional string custom_return_type = 50030;
	E_CustomReturnType = &file_gorums_proto_extTypes[7]
)

var File_gorums_proto protoreflect.FileDescriptor

var file_gorums_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06,
	0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
	0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x3a, 0x32, 0x0a, 0x03, 0x72, 0x70, 0x63, 0x12,
	0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18,
	0xd1, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x03, 0x72, 0x70, 0x63, 0x3a, 0x3a, 0x0a, 0x07,
	0x75, 0x6e, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64,
	0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd2, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x07, 0x75, 0x6e, 0x69, 0x63, 0x61, 0x73, 0x74, 0x3a, 0x3e, 0x0a, 0x09, 0x6d, 0x75, 0x6c, 0x74,
	0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd3, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x6d,
	0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x3a, 0x40, 0x0a, 0x0a, 0x71, 0x75, 0x6f, 0x72,
	0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd4, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a,
	0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x3a, 0x42, 0x0a, 0x0b, 0x63, 0x6f,
	0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68,
	0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xd5, 0x86, 0x03, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x0b, 0x63, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x3a, 0x36,
	0x0a, 0x05, 0x61, 0x73, 0x79, 0x6e, 0x63, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64,
	0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xda, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x05, 0x61, 0x73, 0x79, 0x6e, 0x63, 0x3a, 0x42, 0x0a, 0x0c, 0x70, 0x65, 0x72, 0x5f, 0x6e, 0x6f,
	0x64, 0x65, 0x5f, 0x61, 0x72, 0x67, 0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f,
	0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0xe4, 0x86, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a,
	0x70, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x3a, 0x4e, 0x0a, 0x12, 0x63, 0x75,
	0x73, 0x74, 0x6f, 0x6d, 0x5f, 0x72, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x5f, 0x74, 0x79, 0x70, 0x65,
	0x12, 0x1e, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73,
	0x18, 0xee, 0x86, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x63, 0x75, 0x73, 0x74, 0x6f, 0x6d,
	0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x42, 0x19, 0x5a, 0x17, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x67,
	0x6f, 0x72, 0x75, 0x6d, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_gorums_proto_goTypes = []any{
	(*descriptorpb.MethodOptions)(nil), // 0: google.protobuf.MethodOptions
}
var file_gorums_proto_depIdxs = []int32{
	0, // 0: gorums.rpc:extendee -> google.protobuf.MethodOptions
	0, // 1: gorums.unicast:extendee -> google.protobuf.MethodOptions
	0, // 2: gorums.multicast:extendee -> google.protobuf.MethodOptions
	0, // 3: gorums.quorumcall:extendee -> google.protobuf.MethodOptions
	0, // 4: gorums.correctable:extendee -> google.protobuf.MethodOptions
	0, // 5: gorums.async:extendee -> google.protobuf.MethodOptions
	0, // 6: gorums.per_node_arg:extendee -> google.protobuf.MethodOptions
	0, // 7: gorums.custom_return_type:extendee -> google.protobuf.MethodOptions
	8, // [8:8] is the sub-list for method output_type
	8, // [8:8] is the sub-list for method input_type
	8, // [8:8] is the sub-list for extension type_name
	0, // [0:8] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
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
			NumExtensions: 8,
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
