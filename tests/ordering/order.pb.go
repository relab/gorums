// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.24.0
// 	protoc        v3.12.3
// source: ordering/order.proto

package ordering

import (
	proto "github.com/golang/protobuf/proto"
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

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type Request struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Num uint64 `protobuf:"varint,1,opt,name=Num,proto3" json:"Num,omitempty"`
}

func (x *Request) Reset() {
	*x = Request{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ordering_order_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_ordering_order_proto_msgTypes[0]
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
	return file_ordering_order_proto_rawDescGZIP(), []int{0}
}

func (x *Request) GetNum() uint64 {
	if x != nil {
		return x.Num
	}
	return 0
}

type Response struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	InOrder bool `protobuf:"varint,1,opt,name=InOrder,proto3" json:"InOrder,omitempty"`
}

func (x *Response) Reset() {
	*x = Response{}
	if protoimpl.UnsafeEnabled {
		mi := &file_ordering_order_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Response) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Response) ProtoMessage() {}

func (x *Response) ProtoReflect() protoreflect.Message {
	mi := &file_ordering_order_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
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
	return file_ordering_order_proto_rawDescGZIP(), []int{1}
}

func (x *Response) GetInOrder() bool {
	if x != nil {
		return x.InOrder
	}
	return false
}

var File_ordering_order_proto protoreflect.FileDescriptor

var file_ordering_order_proto_rawDesc = []byte{
	0x0a, 0x14, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2f, 0x6f, 0x72, 0x64, 0x65, 0x72,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67,
	0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x1b,
	0x0a, 0x07, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x4e, 0x75, 0x6d,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x4e, 0x75, 0x6d, 0x22, 0x24, 0x0a, 0x08, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x49, 0x6e, 0x4f, 0x72, 0x64,
	0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x49, 0x6e, 0x4f, 0x72, 0x64, 0x65,
	0x72, 0x32, 0x82, 0x02, 0x0a, 0x0a, 0x47, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x54, 0x65, 0x73, 0x74,
	0x12, 0x35, 0x0a, 0x02, 0x51, 0x43, 0x12, 0x11, 0x2e, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e,
	0x67, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x6f, 0x72, 0x64, 0x65,
	0x72, 0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80,
	0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0x12, 0x3f, 0x0a, 0x08, 0x51, 0x43, 0x46, 0x75, 0x74,
	0x75, 0x72, 0x65, 0x12, 0x11, 0x2e, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2e, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x6f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e,
	0x67, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x0c, 0x80, 0xb5, 0x18, 0x01,
	0x90, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0x12, 0x43, 0x0a, 0x0c, 0x41, 0x73, 0x79, 0x6e,
	0x63, 0x48, 0x61, 0x6e, 0x64, 0x6c, 0x65, 0x72, 0x12, 0x11, 0x2e, 0x6f, 0x72, 0x64, 0x65, 0x72,
	0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x6f, 0x72,
	0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x0c, 0x80, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xb8, 0xb5, 0x18, 0x01, 0x12, 0x37, 0x0a,
	0x08, 0x55, 0x6e, 0x61, 0x72, 0x79, 0x52, 0x50, 0x43, 0x12, 0x11, 0x2e, 0x6f, 0x72, 0x64, 0x65,
	0x72, 0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x6f,
	0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x04, 0x90, 0xb5, 0x18, 0x01, 0x42, 0x3e, 0x5a, 0x3c, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x67, 0x6f, 0x72, 0x75, 0x6d,
	0x73, 0x2f, 0x63, 0x6d, 0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x2d, 0x67, 0x65, 0x6e,
	0x2d, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x73, 0x2f, 0x6f, 0x72,
	0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_ordering_order_proto_rawDescOnce sync.Once
	file_ordering_order_proto_rawDescData = file_ordering_order_proto_rawDesc
)

func file_ordering_order_proto_rawDescGZIP() []byte {
	file_ordering_order_proto_rawDescOnce.Do(func() {
		file_ordering_order_proto_rawDescData = protoimpl.X.CompressGZIP(file_ordering_order_proto_rawDescData)
	})
	return file_ordering_order_proto_rawDescData
}

var file_ordering_order_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_ordering_order_proto_goTypes = []interface{}{
	(*Request)(nil),  // 0: ordering.Request
	(*Response)(nil), // 1: ordering.Response
}
var file_ordering_order_proto_depIdxs = []int32{
	0, // 0: ordering.GorumsTest.QC:input_type -> ordering.Request
	0, // 1: ordering.GorumsTest.QCFuture:input_type -> ordering.Request
	0, // 2: ordering.GorumsTest.AsyncHandler:input_type -> ordering.Request
	0, // 3: ordering.GorumsTest.UnaryRPC:input_type -> ordering.Request
	1, // 4: ordering.GorumsTest.QC:output_type -> ordering.Response
	1, // 5: ordering.GorumsTest.QCFuture:output_type -> ordering.Response
	1, // 6: ordering.GorumsTest.AsyncHandler:output_type -> ordering.Response
	1, // 7: ordering.GorumsTest.UnaryRPC:output_type -> ordering.Response
	4, // [4:8] is the sub-list for method output_type
	0, // [0:4] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_ordering_order_proto_init() }
func file_ordering_order_proto_init() {
	if File_ordering_order_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_ordering_order_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
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
		file_ordering_order_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Response); i {
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
			RawDescriptor: file_ordering_order_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_ordering_order_proto_goTypes,
		DependencyIndexes: file_ordering_order_proto_depIdxs,
		MessageInfos:      file_ordering_order_proto_msgTypes,
	}.Build()
	File_ordering_order_proto = out.File
	file_ordering_order_proto_rawDesc = nil
	file_ordering_order_proto_goTypes = nil
	file_ordering_order_proto_depIdxs = nil
}
