// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.19.2
// source: unresponsive/unresponsive.proto

package unresponsive

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Empty struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *Empty) Reset() {
	*x = Empty{}
	if protoimpl.UnsafeEnabled {
		mi := &file_unresponsive_unresponsive_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Empty) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Empty) ProtoMessage() {}

func (x *Empty) ProtoReflect() protoreflect.Message {
	mi := &file_unresponsive_unresponsive_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Empty.ProtoReflect.Descriptor instead.
func (*Empty) Descriptor() ([]byte, []int) {
	return file_unresponsive_unresponsive_proto_rawDescGZIP(), []int{0}
}

var File_unresponsive_unresponsive_proto protoreflect.FileDescriptor

var file_unresponsive_unresponsive_proto_rawDesc = []byte{
	0x0a, 0x1f, 0x75, 0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x2f, 0x75,
	0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x0c, 0x75, 0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x22,
	0x07, 0x0a, 0x05, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x32, 0x4e, 0x0a, 0x0c, 0x55, 0x6e, 0x72, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x12, 0x3e, 0x0a, 0x10, 0x54, 0x65, 0x73, 0x74,
	0x55, 0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x12, 0x13, 0x2e, 0x75,
	0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x2e, 0x45, 0x6d, 0x70, 0x74,
	0x79, 0x1a, 0x13, 0x2e, 0x75, 0x6e, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x00, 0x42, 0x2c, 0x5a, 0x2a, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x67, 0x6f, 0x72,
	0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x73, 0x2f, 0x75, 0x6e, 0x72, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x69, 0x76, 0x65, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_unresponsive_unresponsive_proto_rawDescOnce sync.Once
	file_unresponsive_unresponsive_proto_rawDescData = file_unresponsive_unresponsive_proto_rawDesc
)

func file_unresponsive_unresponsive_proto_rawDescGZIP() []byte {
	file_unresponsive_unresponsive_proto_rawDescOnce.Do(func() {
		file_unresponsive_unresponsive_proto_rawDescData = protoimpl.X.CompressGZIP(file_unresponsive_unresponsive_proto_rawDescData)
	})
	return file_unresponsive_unresponsive_proto_rawDescData
}

var file_unresponsive_unresponsive_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_unresponsive_unresponsive_proto_goTypes = []interface{}{
	(*Empty)(nil), // 0: unresponsive.Empty
}
var file_unresponsive_unresponsive_proto_depIdxs = []int32{
	0, // 0: unresponsive.Unresponsive.TestUnresponsive:input_type -> unresponsive.Empty
	0, // 1: unresponsive.Unresponsive.TestUnresponsive:output_type -> unresponsive.Empty
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_unresponsive_unresponsive_proto_init() }
func file_unresponsive_unresponsive_proto_init() {
	if File_unresponsive_unresponsive_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_unresponsive_unresponsive_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Empty); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_unresponsive_unresponsive_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_unresponsive_unresponsive_proto_goTypes,
		DependencyIndexes: file_unresponsive_unresponsive_proto_depIdxs,
		MessageInfos:      file_unresponsive_unresponsive_proto_msgTypes,
	}.Build()
	File_unresponsive_unresponsive_proto = out.File
	file_unresponsive_unresponsive_proto_rawDesc = nil
	file_unresponsive_unresponsive_proto_goTypes = nil
	file_unresponsive_unresponsive_proto_depIdxs = nil
}
