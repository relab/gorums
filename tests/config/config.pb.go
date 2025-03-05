// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v5.29.3
// source: config/config.proto

package config

import (
	_ "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Request struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Num           uint64                 `protobuf:"varint,1,opt,name=Num" json:"Num,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Request) Reset() {
	*x = Request{}
	mi := &file_config_config_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_config_config_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Request.ProtoReflect.Descriptor instead.
func (*Request) Descriptor() ([]byte, []int) {
	return file_config_config_proto_rawDescGZIP(), []int{0}
}

func (x *Request) GetNum() uint64 {
	if x != nil {
		return x.Num
	}
	return 0
}

type Response struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Name          string                 `protobuf:"bytes,1,opt,name=Name" json:"Name,omitempty"`
	Num           uint64                 `protobuf:"varint,2,opt,name=Num" json:"Num,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Response) Reset() {
	*x = Response{}
	mi := &file_config_config_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Response) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Response) ProtoMessage() {}

func (x *Response) ProtoReflect() protoreflect.Message {
	mi := &file_config_config_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Response.ProtoReflect.Descriptor instead.
func (*Response) Descriptor() ([]byte, []int) {
	return file_config_config_proto_rawDescGZIP(), []int{1}
}

func (x *Response) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Response) GetNum() uint64 {
	if x != nil {
		return x.Num
	}
	return 0
}

var File_config_config_proto protoreflect.FileDescriptor

var file_config_config_proto_rawDesc = string([]byte{
	0x0a, 0x13, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x1a, 0x0c, 0x67,
	0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x1b, 0x0a, 0x07, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x4e, 0x75, 0x6d, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x03, 0x4e, 0x75, 0x6d, 0x22, 0x30, 0x0a, 0x08, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x4e, 0x75, 0x6d, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x4e, 0x75, 0x6d, 0x32, 0x3f, 0x0a, 0x0a, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x54, 0x65, 0x73, 0x74, 0x12, 0x31, 0x0a, 0x06, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x12, 0x0f, 0x2e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x10, 0x2e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x42, 0x2b, 0x5a, 0x24, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f,
	0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x73, 0x2f, 0x63, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x92, 0x03, 0x02, 0x08, 0x02, 0x62, 0x08, 0x65, 0x64, 0x69, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x70, 0xe8, 0x07,
})

var (
	file_config_config_proto_rawDescOnce sync.Once
	file_config_config_proto_rawDescData []byte
)

func file_config_config_proto_rawDescGZIP() []byte {
	file_config_config_proto_rawDescOnce.Do(func() {
		file_config_config_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_config_config_proto_rawDesc), len(file_config_config_proto_rawDesc)))
	})
	return file_config_config_proto_rawDescData
}

var file_config_config_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_config_config_proto_goTypes = []any{
	(*Request)(nil),  // 0: config.Request
	(*Response)(nil), // 1: config.Response
}
var file_config_config_proto_depIdxs = []int32{
	0, // 0: config.ConfigTest.Config:input_type -> config.Request
	1, // 1: config.ConfigTest.Config:output_type -> config.Response
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_config_config_proto_init() }
func file_config_config_proto_init() {
	if File_config_config_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_config_config_proto_rawDesc), len(file_config_config_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_config_config_proto_goTypes,
		DependencyIndexes: file_config_config_proto_depIdxs,
		MessageInfos:      file_config_config_proto_msgTypes,
	}.Build()
	File_config_config_proto = out.File
	file_config_config_proto_goTypes = nil
	file_config_config_proto_depIdxs = nil
}
