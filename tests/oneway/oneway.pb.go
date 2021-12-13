// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.17.3
// source: oneway/oneway.proto

package oneway

import (
	_ "github.com/relab/gorums"
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

type Request struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Num uint64 `protobuf:"varint,1,opt,name=Num,proto3" json:"Num,omitempty"`
}

func (x *Request) Reset() {
	*x = Request{}
	if protoimpl.UnsafeEnabled {
		mi := &file_oneway_oneway_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_oneway_oneway_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
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
	return file_oneway_oneway_proto_rawDescGZIP(), []int{0}
}

func (x *Request) GetNum() uint64 {
	if x != nil {
		return x.Num
	}
	return 0
}

type Empty struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *Empty) Reset() {
	*x = Empty{}
	if protoimpl.UnsafeEnabled {
		mi := &file_oneway_oneway_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Empty) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Empty) ProtoMessage() {}

func (x *Empty) ProtoReflect() protoreflect.Message {
	mi := &file_oneway_oneway_proto_msgTypes[1]
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
	return file_oneway_oneway_proto_rawDescGZIP(), []int{1}
}

var File_oneway_oneway_proto protoreflect.FileDescriptor

var file_oneway_oneway_proto_rawDesc = []byte{
	0x0a, 0x13, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x2f, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x1a, 0x0c, 0x67,
	0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x1b, 0x0a, 0x07, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x4e, 0x75, 0x6d, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x03, 0x4e, 0x75, 0x6d, 0x22, 0x07, 0x0a, 0x05, 0x45, 0x6d, 0x70, 0x74,
	0x79, 0x32, 0xae, 0x01, 0x0a, 0x0a, 0x4f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x54, 0x65, 0x73, 0x74,
	0x12, 0x2f, 0x0a, 0x07, 0x55, 0x6e, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x0f, 0x2e, 0x6f, 0x6e,
	0x65, 0x77, 0x61, 0x79, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x6f,
	0x6e, 0x65, 0x77, 0x61, 0x79, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x90, 0xb5, 0x18,
	0x01, 0x12, 0x31, 0x0a, 0x09, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x0f,
	0x2e, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x0d, 0x2e, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04,
	0x98, 0xb5, 0x18, 0x01, 0x12, 0x3c, 0x0a, 0x10, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73,
	0x74, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x12, 0x0f, 0x2e, 0x6f, 0x6e, 0x65, 0x77, 0x61,
	0x79, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x6f, 0x6e, 0x65, 0x77,
	0x61, 0x79, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x08, 0x98, 0xb5, 0x18, 0x01, 0xa0, 0xb6,
	0x18, 0x01, 0x42, 0x26, 0x5a, 0x24, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d,
	0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65,
	0x73, 0x74, 0x73, 0x2f, 0x6f, 0x6e, 0x65, 0x77, 0x61, 0x79, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
}

var (
	file_oneway_oneway_proto_rawDescOnce sync.Once
	file_oneway_oneway_proto_rawDescData = file_oneway_oneway_proto_rawDesc
)

func file_oneway_oneway_proto_rawDescGZIP() []byte {
	file_oneway_oneway_proto_rawDescOnce.Do(func() {
		file_oneway_oneway_proto_rawDescData = protoimpl.X.CompressGZIP(file_oneway_oneway_proto_rawDescData)
	})
	return file_oneway_oneway_proto_rawDescData
}

var file_oneway_oneway_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_oneway_oneway_proto_goTypes = []interface{}{
	(*Request)(nil), // 0: oneway.Request
	(*Empty)(nil),   // 1: oneway.Empty
}
var file_oneway_oneway_proto_depIdxs = []int32{
	0, // 0: oneway.OnewayTest.Unicast:input_type -> oneway.Request
	0, // 1: oneway.OnewayTest.Multicast:input_type -> oneway.Request
	0, // 2: oneway.OnewayTest.MulticastPerNode:input_type -> oneway.Request
	1, // 3: oneway.OnewayTest.Unicast:output_type -> oneway.Empty
	1, // 4: oneway.OnewayTest.Multicast:output_type -> oneway.Empty
	1, // 5: oneway.OnewayTest.MulticastPerNode:output_type -> oneway.Empty
	3, // [3:6] is the sub-list for method output_type
	0, // [0:3] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_oneway_oneway_proto_init() }
func file_oneway_oneway_proto_init() {
	if File_oneway_oneway_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_oneway_oneway_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Request); i {
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
		file_oneway_oneway_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
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
			RawDescriptor: file_oneway_oneway_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_oneway_oneway_proto_goTypes,
		DependencyIndexes: file_oneway_oneway_proto_depIdxs,
		MessageInfos:      file_oneway_oneway_proto_msgTypes,
	}.Build()
	File_oneway_oneway_proto = out.File
	file_oneway_oneway_proto_rawDesc = nil
	file_oneway_oneway_proto_goTypes = nil
	file_oneway_oneway_proto_depIdxs = nil
}
