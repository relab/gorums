// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.19.0-devel
// 	protoc        v3.11.4
// source: cmd/protoc-gen-gorums/tests/quorumcall/quorumcall.proto

package quorumcall

import (
	_ "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(19 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 19)
)

type ReadRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Value string `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
}

func (x *ReadRequest) Reset() {
	*x = ReadRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadRequest) ProtoMessage() {}

func (x *ReadRequest) ProtoReflect() protoreflect.Message {
	mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadRequest.ProtoReflect.Descriptor instead.
func (*ReadRequest) Descriptor() ([]byte, []int) {
	return file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescGZIP(), []int{0}
}

func (x *ReadRequest) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type ReadResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Result int64 `protobuf:"varint,1,opt,name=Result,proto3" json:"Result,omitempty"`
}

func (x *ReadResponse) Reset() {
	*x = ReadResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadResponse) ProtoMessage() {}

func (x *ReadResponse) ProtoReflect() protoreflect.Message {
	mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadResponse.ProtoReflect.Descriptor instead.
func (*ReadResponse) Descriptor() ([]byte, []int) {
	return file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescGZIP(), []int{1}
}

func (x *ReadResponse) GetResult() int64 {
	if x != nil {
		return x.Result
	}
	return 0
}

type MyReadResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Value  string `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
	Result int64  `protobuf:"varint,2,opt,name=Result,proto3" json:"Result,omitempty"`
}

func (x *MyReadResponse) Reset() {
	*x = MyReadResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MyReadResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MyReadResponse) ProtoMessage() {}

func (x *MyReadResponse) ProtoReflect() protoreflect.Message {
	mi := &file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MyReadResponse.ProtoReflect.Descriptor instead.
func (*MyReadResponse) Descriptor() ([]byte, []int) {
	return file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescGZIP(), []int{2}
}

func (x *MyReadResponse) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

func (x *MyReadResponse) GetResult() int64 {
	if x != nil {
		return x.Result
	}
	return 0
}

var File_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto protoreflect.FileDescriptor

var file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDesc = []byte{
	0x0a, 0x37, 0x63, 0x6d, 0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x2d, 0x67, 0x65, 0x6e,
	0x2d, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x73, 0x2f, 0x71, 0x75,
	0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2f, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63,
	0x61, 0x6c, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0a, 0x71, 0x75, 0x6f, 0x72, 0x75,
	0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x23, 0x0a, 0x0b, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x22, 0x26, 0x0a, 0x0c, 0x52, 0x65, 0x61, 0x64,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x52, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x22, 0x3e, 0x0a, 0x0e, 0x4d, 0x79, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x52, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x32, 0xf9, 0x06, 0x0a, 0x0d, 0x52, 0x65, 0x61, 0x64, 0x65, 0x72, 0x53, 0x65, 0x72, 0x76, 0x69,
	0x63, 0x65, 0x12, 0x3f, 0x0a, 0x08, 0x52, 0x65, 0x61, 0x64, 0x47, 0x72, 0x70, 0x63, 0x12, 0x17,
	0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d,
	0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x00, 0x12, 0x49, 0x0a, 0x0e, 0x52, 0x65, 0x61, 0x64, 0x51, 0x75, 0x6f, 0x72, 0x75,
	0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61,
	0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18,
	0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x80, 0xb5, 0x18, 0x01, 0x12, 0x57,
	0x0a, 0x18, 0x52, 0x65, 0x61, 0x64, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c,
	0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f,
	0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c,
	0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80,
	0xb5, 0x18, 0x01, 0xb0, 0xb5, 0x18, 0x01, 0x12, 0x5d, 0x0a, 0x1e, 0x52, 0x65, 0x61, 0x64, 0x51,
	0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x51, 0x46, 0x57, 0x69, 0x74, 0x68, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x41, 0x72, 0x67, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f, 0x72,
	0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e,
	0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5,
	0x18, 0x01, 0xa8, 0xb5, 0x18, 0x01, 0x12, 0x6b, 0x0a, 0x1e, 0x52, 0x65, 0x61, 0x64, 0x51, 0x75,
	0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x52, 0x65,
	0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75,
	0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52,
	0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x16, 0x80, 0xb5, 0x18,
	0x01, 0xc2, 0xf3, 0x18, 0x0e, 0x4d, 0x79, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x68, 0x0a, 0x13, 0x52, 0x65, 0x61, 0x64, 0x51, 0x75, 0x6f, 0x72, 0x75,
	0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x43, 0x6f, 0x6d, 0x62, 0x6f, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f,
	0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c,
	0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1e, 0x80,
	0xb5, 0x18, 0x01, 0xa8, 0xb5, 0x18, 0x01, 0xb0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0e, 0x4d,
	0x79, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4a, 0x0a,
	0x0d, 0x52, 0x65, 0x61, 0x64, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x17,
	0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d,
	0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01, 0x28, 0x01, 0x12, 0x4f, 0x0a, 0x14, 0x52, 0x65, 0x61,
	0x64, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74, 0x75, 0x72,
	0x65, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52,
	0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f,
	0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x5c, 0x0a, 0x0f, 0x52, 0x65,
	0x61, 0x64, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x12, 0x17, 0x2e,
	0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63,
	0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x16, 0x88, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0e, 0x4d, 0x79, 0x52, 0x65, 0x61, 0x64,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x52, 0x0a, 0x15, 0x52, 0x65, 0x61, 0x64,
	0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53, 0x74, 0x72, 0x65, 0x61,
	0x6d, 0x12, 0x17, 0x2e, 0x71, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52,
	0x65, 0x61, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x71, 0x75, 0x6f,
	0x72, 0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x90, 0xb5, 0x18, 0x01, 0x30, 0x01, 0x42, 0x28, 0x5a, 0x26,
	0x63, 0x6d, 0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x2d, 0x67, 0x65, 0x6e, 0x2d, 0x67,
	0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x73, 0x2f, 0x71, 0x75, 0x6f, 0x72,
	0x75, 0x6d, 0x63, 0x61, 0x6c, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescOnce sync.Once
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescData = file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDesc
)

func file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescGZIP() []byte {
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescOnce.Do(func() {
		file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescData = protoimpl.X.CompressGZIP(file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescData)
	})
	return file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDescData
}

var file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_goTypes = []interface{}{
	(*ReadRequest)(nil),    // 0: quorumcall.ReadRequest
	(*ReadResponse)(nil),   // 1: quorumcall.ReadResponse
	(*MyReadResponse)(nil), // 2: quorumcall.MyReadResponse
}
var file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_depIdxs = []int32{
	0,  // 0: quorumcall.ReaderService.ReadGrpc:input_type -> quorumcall.ReadRequest
	0,  // 1: quorumcall.ReaderService.ReadQuorumCall:input_type -> quorumcall.ReadRequest
	0,  // 2: quorumcall.ReaderService.ReadQuorumCallPerNodeArg:input_type -> quorumcall.ReadRequest
	0,  // 3: quorumcall.ReaderService.ReadQuorumCallQFWithRequestArg:input_type -> quorumcall.ReadRequest
	0,  // 4: quorumcall.ReaderService.ReadQuorumCallCustomReturnType:input_type -> quorumcall.ReadRequest
	0,  // 5: quorumcall.ReaderService.ReadQuorumCallCombo:input_type -> quorumcall.ReadRequest
	0,  // 6: quorumcall.ReaderService.ReadMulticast:input_type -> quorumcall.ReadRequest
	0,  // 7: quorumcall.ReaderService.ReadQuorumCallFuture:input_type -> quorumcall.ReadRequest
	0,  // 8: quorumcall.ReaderService.ReadCorrectable:input_type -> quorumcall.ReadRequest
	0,  // 9: quorumcall.ReaderService.ReadCorrectableStream:input_type -> quorumcall.ReadRequest
	1,  // 10: quorumcall.ReaderService.ReadGrpc:output_type -> quorumcall.ReadResponse
	1,  // 11: quorumcall.ReaderService.ReadQuorumCall:output_type -> quorumcall.ReadResponse
	1,  // 12: quorumcall.ReaderService.ReadQuorumCallPerNodeArg:output_type -> quorumcall.ReadResponse
	1,  // 13: quorumcall.ReaderService.ReadQuorumCallQFWithRequestArg:output_type -> quorumcall.ReadResponse
	1,  // 14: quorumcall.ReaderService.ReadQuorumCallCustomReturnType:output_type -> quorumcall.ReadResponse
	1,  // 15: quorumcall.ReaderService.ReadQuorumCallCombo:output_type -> quorumcall.ReadResponse
	1,  // 16: quorumcall.ReaderService.ReadMulticast:output_type -> quorumcall.ReadResponse
	1,  // 17: quorumcall.ReaderService.ReadQuorumCallFuture:output_type -> quorumcall.ReadResponse
	1,  // 18: quorumcall.ReaderService.ReadCorrectable:output_type -> quorumcall.ReadResponse
	1,  // 19: quorumcall.ReaderService.ReadCorrectableStream:output_type -> quorumcall.ReadResponse
	10, // [10:20] is the sub-list for method output_type
	0,  // [0:10] is the sub-list for method input_type
	0,  // [0:0] is the sub-list for extension type_name
	0,  // [0:0] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() { file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_init() }
func file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_init() {
	if File_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadRequest); i {
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
		file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadResponse); i {
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
		file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MyReadResponse); i {
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
			RawDescriptor: file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_goTypes,
		DependencyIndexes: file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_depIdxs,
		MessageInfos:      file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_msgTypes,
	}.Build()
	File_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto = out.File
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_rawDesc = nil
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_goTypes = nil
	file_cmd_protoc_gen_gorums_tests_quorumcall_quorumcall_proto_depIdxs = nil
}