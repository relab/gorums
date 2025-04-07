// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v5.29.3
// source: gorums.proto

package gorums

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	reflect "reflect"
	unsafe "unsafe"
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

const file_gorums_proto_rawDesc = "" +
	"\n" +
	"\fgorums.proto\x12\x06gorums\x1a google/protobuf/descriptor.proto:2\n" +
	"\x03rpc\x12\x1e.google.protobuf.MethodOptions\x18ц\x03 \x01(\bR\x03rpc::\n" +
	"\aunicast\x12\x1e.google.protobuf.MethodOptions\x18҆\x03 \x01(\bR\aunicast:>\n" +
	"\tmulticast\x12\x1e.google.protobuf.MethodOptions\x18ӆ\x03 \x01(\bR\tmulticast:@\n" +
	"\n" +
	"quorumcall\x12\x1e.google.protobuf.MethodOptions\x18Ԇ\x03 \x01(\bR\n" +
	"quorumcall:B\n" +
	"\vcorrectable\x12\x1e.google.protobuf.MethodOptions\x18Ն\x03 \x01(\bR\vcorrectable:6\n" +
	"\x05async\x12\x1e.google.protobuf.MethodOptions\x18چ\x03 \x01(\bR\x05async:B\n" +
	"\fper_node_arg\x12\x1e.google.protobuf.MethodOptions\x18\xe4\x86\x03 \x01(\bR\n" +
	"perNodeArg:N\n" +
	"\x12custom_return_type\x12\x1e.google.protobuf.MethodOptions\x18\xee\x86\x03 \x01(\tR\x10customReturnTypeB\x1eZ\x17github.com/relab/gorums\x92\x03\x02\b\x02b\beditionsp\xe8\a"

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
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_gorums_proto_rawDesc), len(file_gorums_proto_rawDesc)),
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
	file_gorums_proto_goTypes = nil
	file_gorums_proto_depIdxs = nil
}
