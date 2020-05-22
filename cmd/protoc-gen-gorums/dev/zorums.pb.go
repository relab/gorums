// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.23.0
// 	protoc        v3.11.4
// source: zorums.proto

package dev

import (
	proto "github.com/golang/protobuf/proto"
	empty "github.com/golang/protobuf/ptypes/empty"
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

	Value string `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
}

func (x *Request) Reset() {
	*x = Request{}
	if protoimpl.UnsafeEnabled {
		mi := &file_zorums_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Request) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Request) ProtoMessage() {}

func (x *Request) ProtoReflect() protoreflect.Message {
	mi := &file_zorums_proto_msgTypes[0]
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
	return file_zorums_proto_rawDescGZIP(), []int{0}
}

func (x *Request) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

type Response struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Result int64 `protobuf:"varint,1,opt,name=Result,proto3" json:"Result,omitempty"`
}

func (x *Response) Reset() {
	*x = Response{}
	if protoimpl.UnsafeEnabled {
		mi := &file_zorums_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Response) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Response) ProtoMessage() {}

func (x *Response) ProtoReflect() protoreflect.Message {
	mi := &file_zorums_proto_msgTypes[1]
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
	return file_zorums_proto_rawDescGZIP(), []int{1}
}

func (x *Response) GetResult() int64 {
	if x != nil {
		return x.Result
	}
	return 0
}

type MyResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Value string `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
}

func (x *MyResponse) Reset() {
	*x = MyResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_zorums_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MyResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MyResponse) ProtoMessage() {}

func (x *MyResponse) ProtoReflect() protoreflect.Message {
	mi := &file_zorums_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MyResponse.ProtoReflect.Descriptor instead.
func (*MyResponse) Descriptor() ([]byte, []int) {
	return file_zorums_proto_rawDescGZIP(), []int{2}
}

func (x *MyResponse) GetValue() string {
	if x != nil {
		return x.Value
	}
	return ""
}

var File_zorums_proto protoreflect.FileDescriptor

var file_zorums_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x7a, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03,
	0x64, 0x65, 0x76, 0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x1f,
	0x0a, 0x07, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x56, 0x61, 0x6c,
	0x75, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x22,
	0x22, 0x0a, 0x08, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x22, 0x22, 0x0a, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x14, 0x0a, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x05, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x32, 0xb0, 0x15, 0x0a, 0x0d, 0x5a, 0x6f, 0x72, 0x75,
	0x6d, 0x73, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x29, 0x0a, 0x08, 0x47, 0x52, 0x50,
	0x43, 0x43, 0x61, 0x6c, 0x6c, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x12, 0x2f, 0x0a, 0x0a, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61,
	0x6c, 0x6c, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x04, 0x80, 0xb5, 0x18, 0x01, 0x12, 0x3d, 0x0a, 0x14, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43,
	0x61, 0x6c, 0x6c, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e,
	0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5, 0x18, 0x01,
	0xd0, 0xb5, 0x18, 0x01, 0x12, 0x4d, 0x0a, 0x1a, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61,
	0x6c, 0x6c, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79,
	0x70, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x12, 0x80, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x46, 0x0a, 0x0f, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c,
	0x6c, 0x43, 0x6f, 0x6d, 0x62, 0x6f, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x16, 0x80, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18,
	0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a, 0x0f, 0x51,
	0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x80, 0xb5, 0x18, 0x01, 0x12, 0x3e, 0x0a, 0x10, 0x51,
	0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x32, 0x12,
	0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x80, 0xb5, 0x18, 0x01, 0x12, 0x30, 0x0a, 0x09, 0x4d,
	0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01, 0x28, 0x01, 0x12, 0x3e, 0x0a,
	0x13, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64,
	0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x08, 0x98, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0x28, 0x01, 0x12, 0x31, 0x0a,
	0x0a, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x32, 0x12, 0x0c, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01, 0x28, 0x01,
	0x12, 0x3a, 0x0a, 0x0a, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x33, 0x12, 0x0c,
	0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45,
	0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01, 0x28, 0x01, 0x12, 0x44, 0x0a, 0x0a,
	0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74, 0x34, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70,
	0x74, 0x79, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x98, 0xb5, 0x18, 0x01,
	0x28, 0x01, 0x12, 0x39, 0x0a, 0x10, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c,
	0x46, 0x75, 0x74, 0x75, 0x72, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0x12, 0x47, 0x0a,
	0x1a, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74, 0x75, 0x72,
	0x65, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x0c, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5,
	0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0x12, 0x57, 0x0a, 0x20, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d,
	0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74, 0x75, 0x72, 0x65, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d,
	0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76,
	0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x16, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18,
	0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x50, 0x0a, 0x15, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74,
	0x75, 0x72, 0x65, 0x43, 0x6f, 0x6d, 0x62, 0x6f, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1a, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0xd0,
	0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x3a, 0x0a, 0x11, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x46,
	0x75, 0x74, 0x75, 0x72, 0x65, 0x32, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0x12, 0x47, 0x0a,
	0x15, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74, 0x75, 0x72,
	0x65, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x08, 0x80, 0xb5,
	0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0x12, 0x49, 0x0a, 0x16, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d,
	0x43, 0x61, 0x6c, 0x6c, 0x46, 0x75, 0x74, 0x75, 0x72, 0x65, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x32,
	0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18,
	0x01, 0x12, 0x30, 0x0a, 0x0b, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65,
	0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d,
	0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0,
	0xb5, 0x18, 0x01, 0x12, 0x3e, 0x0a, 0x15, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62,
	0x6c, 0x65, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64,
	0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76,
	0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0xa0, 0xb5, 0x18, 0x01, 0xd0,
	0xb5, 0x18, 0x01, 0x12, 0x4e, 0x0a, 0x1b, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62,
	0x6c, 0x65, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79,
	0x70, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x12, 0xa0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x47, 0x0a, 0x10, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62,
	0x6c, 0x65, 0x43, 0x6f, 0x6d, 0x62, 0x6f, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x16, 0xa0, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0xc2, 0xf3,
	0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a, 0x10,
	0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x45, 0x6d, 0x70, 0x74, 0x79,
	0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x40, 0x0a, 0x11,
	0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x45, 0x6d, 0x70, 0x74, 0x79,
	0x32, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x38,
	0x0a, 0x11, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x30, 0x01, 0x12, 0x46, 0x0a, 0x1b, 0x43, 0x6f, 0x72, 0x72,
	0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x50, 0x65, 0x72,
	0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0xa0, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0x30, 0x01,
	0x12, 0x56, 0x0a, 0x21, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53,
	0x74, 0x72, 0x65, 0x61, 0x6d, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x52, 0x65, 0x74, 0x75, 0x72,
	0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x12, 0xa0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x30, 0x01, 0x12, 0x4f, 0x0a, 0x16, 0x43, 0x6f, 0x72, 0x72,
	0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x43, 0x6f, 0x6d,
	0x62, 0x6f, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x16, 0xa0, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x30, 0x01, 0x12, 0x46, 0x0a, 0x16, 0x43, 0x6f, 0x72,
	0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x45, 0x6d,
	0x70, 0x74, 0x79, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x30,
	0x01, 0x12, 0x48, 0x0a, 0x17, 0x43, 0x6f, 0x72, 0x72, 0x65, 0x63, 0x74, 0x61, 0x62, 0x6c, 0x65,
	0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x32, 0x12, 0x16, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45,
	0x6d, 0x70, 0x74, 0x79, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x30, 0x01, 0x12, 0x33, 0x0a, 0x0a, 0x4f,
	0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x51, 0x43, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x08, 0x80, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01,
	0x12, 0x3f, 0x0a, 0x12, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x50, 0x65, 0x72, 0x4e,
	0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x22, 0x0c, 0x80, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18,
	0x01, 0x12, 0x4f, 0x0a, 0x18, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x43, 0x75, 0x73,
	0x74, 0x6f, 0x6d, 0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0c, 0x2e,
	0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x16, 0x80, 0xb5, 0x18, 0x01,
	0x90, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x48, 0x0a, 0x0d, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x43, 0x6f,
	0x6d, 0x62, 0x6f, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x1a, 0x80, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0xc2, 0xf3,
	0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x35, 0x0a, 0x10,
	0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x55, 0x6e, 0x61, 0x72, 0x79, 0x52, 0x50, 0x43,
	0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d,
	0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0x90,
	0xb5, 0x18, 0x01, 0x12, 0x3b, 0x0a, 0x0e, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x46,
	0x75, 0x74, 0x75, 0x72, 0x65, 0x12, 0x0c, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x0c, 0x80, 0xb5, 0x18, 0x01, 0x88, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01,
	0x12, 0x49, 0x0a, 0x18, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x46, 0x75, 0x74, 0x75,
	0x72, 0x65, 0x50, 0x65, 0x72, 0x4e, 0x6f, 0x64, 0x65, 0x41, 0x72, 0x67, 0x12, 0x0c, 0x2e, 0x64,
	0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65, 0x76,
	0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x10, 0x80, 0xb5, 0x18, 0x01, 0x88,
	0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0x12, 0x59, 0x0a, 0x1e, 0x4f,
	0x72, 0x64, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x46, 0x75, 0x74, 0x75, 0x72, 0x65, 0x43, 0x75, 0x73,
	0x74, 0x6f, 0x6d, 0x52, 0x65, 0x74, 0x75, 0x72, 0x6e, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0c, 0x2e,
	0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1a, 0x80, 0xb5, 0x18, 0x01,
	0x88, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a, 0x4d, 0x79, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x52, 0x0a, 0x13, 0x4f, 0x72, 0x64, 0x65, 0x72, 0x69,
	0x6e, 0x67, 0x46, 0x75, 0x74, 0x75, 0x72, 0x65, 0x43, 0x6f, 0x6d, 0x62, 0x6f, 0x12, 0x0c, 0x2e,
	0x64, 0x65, 0x76, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0d, 0x2e, 0x64, 0x65,
	0x76, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x1e, 0x80, 0xb5, 0x18, 0x01,
	0x88, 0xb5, 0x18, 0x01, 0x90, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0xc2, 0xf3, 0x18, 0x0a,
	0x4d, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x1b, 0x5a, 0x19, 0x63, 0x6d,
	0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x63, 0x2d, 0x67, 0x65, 0x6e, 0x2d, 0x67, 0x6f, 0x72,
	0x75, 0x6d, 0x73, 0x2f, 0x64, 0x65, 0x76, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_zorums_proto_rawDescOnce sync.Once
	file_zorums_proto_rawDescData = file_zorums_proto_rawDesc
)

func file_zorums_proto_rawDescGZIP() []byte {
	file_zorums_proto_rawDescOnce.Do(func() {
		file_zorums_proto_rawDescData = protoimpl.X.CompressGZIP(file_zorums_proto_rawDescData)
	})
	return file_zorums_proto_rawDescData
}

var file_zorums_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_zorums_proto_goTypes = []interface{}{
	(*Request)(nil),     // 0: dev.Request
	(*Response)(nil),    // 1: dev.Response
	(*MyResponse)(nil),  // 2: dev.MyResponse
	(*empty.Empty)(nil), // 3: google.protobuf.Empty
}
var file_zorums_proto_depIdxs = []int32{
	0,  // 0: dev.ZorumsService.GRPCCall:input_type -> dev.Request
	0,  // 1: dev.ZorumsService.QuorumCall:input_type -> dev.Request
	0,  // 2: dev.ZorumsService.QuorumCallPerNodeArg:input_type -> dev.Request
	0,  // 3: dev.ZorumsService.QuorumCallCustomReturnType:input_type -> dev.Request
	0,  // 4: dev.ZorumsService.QuorumCallCombo:input_type -> dev.Request
	3,  // 5: dev.ZorumsService.QuorumCallEmpty:input_type -> google.protobuf.Empty
	0,  // 6: dev.ZorumsService.QuorumCallEmpty2:input_type -> dev.Request
	0,  // 7: dev.ZorumsService.Multicast:input_type -> dev.Request
	0,  // 8: dev.ZorumsService.MulticastPerNodeArg:input_type -> dev.Request
	0,  // 9: dev.ZorumsService.Multicast2:input_type -> dev.Request
	0,  // 10: dev.ZorumsService.Multicast3:input_type -> dev.Request
	3,  // 11: dev.ZorumsService.Multicast4:input_type -> google.protobuf.Empty
	0,  // 12: dev.ZorumsService.QuorumCallFuture:input_type -> dev.Request
	0,  // 13: dev.ZorumsService.QuorumCallFuturePerNodeArg:input_type -> dev.Request
	0,  // 14: dev.ZorumsService.QuorumCallFutureCustomReturnType:input_type -> dev.Request
	0,  // 15: dev.ZorumsService.QuorumCallFutureCombo:input_type -> dev.Request
	0,  // 16: dev.ZorumsService.QuorumCallFuture2:input_type -> dev.Request
	0,  // 17: dev.ZorumsService.QuorumCallFutureEmpty:input_type -> dev.Request
	3,  // 18: dev.ZorumsService.QuorumCallFutureEmpty2:input_type -> google.protobuf.Empty
	0,  // 19: dev.ZorumsService.Correctable:input_type -> dev.Request
	0,  // 20: dev.ZorumsService.CorrectablePerNodeArg:input_type -> dev.Request
	0,  // 21: dev.ZorumsService.CorrectableCustomReturnType:input_type -> dev.Request
	0,  // 22: dev.ZorumsService.CorrectableCombo:input_type -> dev.Request
	0,  // 23: dev.ZorumsService.CorrectableEmpty:input_type -> dev.Request
	3,  // 24: dev.ZorumsService.CorrectableEmpty2:input_type -> google.protobuf.Empty
	0,  // 25: dev.ZorumsService.CorrectableStream:input_type -> dev.Request
	0,  // 26: dev.ZorumsService.CorrectableStreamPerNodeArg:input_type -> dev.Request
	0,  // 27: dev.ZorumsService.CorrectableStreamCustomReturnType:input_type -> dev.Request
	0,  // 28: dev.ZorumsService.CorrectableStreamCombo:input_type -> dev.Request
	0,  // 29: dev.ZorumsService.CorrectableStreamEmpty:input_type -> dev.Request
	3,  // 30: dev.ZorumsService.CorrectableStreamEmpty2:input_type -> google.protobuf.Empty
	0,  // 31: dev.ZorumsService.OrderingQC:input_type -> dev.Request
	0,  // 32: dev.ZorumsService.OrderingPerNodeArg:input_type -> dev.Request
	0,  // 33: dev.ZorumsService.OrderingCustomReturnType:input_type -> dev.Request
	0,  // 34: dev.ZorumsService.OrderingCombo:input_type -> dev.Request
	0,  // 35: dev.ZorumsService.OrderingUnaryRPC:input_type -> dev.Request
	0,  // 36: dev.ZorumsService.OrderingFuture:input_type -> dev.Request
	0,  // 37: dev.ZorumsService.OrderingFuturePerNodeArg:input_type -> dev.Request
	0,  // 38: dev.ZorumsService.OrderingFutureCustomReturnType:input_type -> dev.Request
	0,  // 39: dev.ZorumsService.OrderingFutureCombo:input_type -> dev.Request
	1,  // 40: dev.ZorumsService.GRPCCall:output_type -> dev.Response
	1,  // 41: dev.ZorumsService.QuorumCall:output_type -> dev.Response
	1,  // 42: dev.ZorumsService.QuorumCallPerNodeArg:output_type -> dev.Response
	1,  // 43: dev.ZorumsService.QuorumCallCustomReturnType:output_type -> dev.Response
	1,  // 44: dev.ZorumsService.QuorumCallCombo:output_type -> dev.Response
	1,  // 45: dev.ZorumsService.QuorumCallEmpty:output_type -> dev.Response
	3,  // 46: dev.ZorumsService.QuorumCallEmpty2:output_type -> google.protobuf.Empty
	1,  // 47: dev.ZorumsService.Multicast:output_type -> dev.Response
	1,  // 48: dev.ZorumsService.MulticastPerNodeArg:output_type -> dev.Response
	1,  // 49: dev.ZorumsService.Multicast2:output_type -> dev.Response
	3,  // 50: dev.ZorumsService.Multicast3:output_type -> google.protobuf.Empty
	3,  // 51: dev.ZorumsService.Multicast4:output_type -> google.protobuf.Empty
	1,  // 52: dev.ZorumsService.QuorumCallFuture:output_type -> dev.Response
	1,  // 53: dev.ZorumsService.QuorumCallFuturePerNodeArg:output_type -> dev.Response
	1,  // 54: dev.ZorumsService.QuorumCallFutureCustomReturnType:output_type -> dev.Response
	1,  // 55: dev.ZorumsService.QuorumCallFutureCombo:output_type -> dev.Response
	1,  // 56: dev.ZorumsService.QuorumCallFuture2:output_type -> dev.Response
	3,  // 57: dev.ZorumsService.QuorumCallFutureEmpty:output_type -> google.protobuf.Empty
	1,  // 58: dev.ZorumsService.QuorumCallFutureEmpty2:output_type -> dev.Response
	1,  // 59: dev.ZorumsService.Correctable:output_type -> dev.Response
	1,  // 60: dev.ZorumsService.CorrectablePerNodeArg:output_type -> dev.Response
	1,  // 61: dev.ZorumsService.CorrectableCustomReturnType:output_type -> dev.Response
	1,  // 62: dev.ZorumsService.CorrectableCombo:output_type -> dev.Response
	3,  // 63: dev.ZorumsService.CorrectableEmpty:output_type -> google.protobuf.Empty
	1,  // 64: dev.ZorumsService.CorrectableEmpty2:output_type -> dev.Response
	1,  // 65: dev.ZorumsService.CorrectableStream:output_type -> dev.Response
	1,  // 66: dev.ZorumsService.CorrectableStreamPerNodeArg:output_type -> dev.Response
	1,  // 67: dev.ZorumsService.CorrectableStreamCustomReturnType:output_type -> dev.Response
	1,  // 68: dev.ZorumsService.CorrectableStreamCombo:output_type -> dev.Response
	3,  // 69: dev.ZorumsService.CorrectableStreamEmpty:output_type -> google.protobuf.Empty
	1,  // 70: dev.ZorumsService.CorrectableStreamEmpty2:output_type -> dev.Response
	1,  // 71: dev.ZorumsService.OrderingQC:output_type -> dev.Response
	1,  // 72: dev.ZorumsService.OrderingPerNodeArg:output_type -> dev.Response
	1,  // 73: dev.ZorumsService.OrderingCustomReturnType:output_type -> dev.Response
	1,  // 74: dev.ZorumsService.OrderingCombo:output_type -> dev.Response
	1,  // 75: dev.ZorumsService.OrderingUnaryRPC:output_type -> dev.Response
	1,  // 76: dev.ZorumsService.OrderingFuture:output_type -> dev.Response
	1,  // 77: dev.ZorumsService.OrderingFuturePerNodeArg:output_type -> dev.Response
	1,  // 78: dev.ZorumsService.OrderingFutureCustomReturnType:output_type -> dev.Response
	1,  // 79: dev.ZorumsService.OrderingFutureCombo:output_type -> dev.Response
	40, // [40:80] is the sub-list for method output_type
	0,  // [0:40] is the sub-list for method input_type
	0,  // [0:0] is the sub-list for extension type_name
	0,  // [0:0] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() { file_zorums_proto_init() }
func file_zorums_proto_init() {
	if File_zorums_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_zorums_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
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
		file_zorums_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
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
		file_zorums_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MyResponse); i {
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
			RawDescriptor: file_zorums_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_zorums_proto_goTypes,
		DependencyIndexes: file_zorums_proto_depIdxs,
		MessageInfos:      file_zorums_proto_msgTypes,
	}.Build()
	File_zorums_proto = out.File
	file_zorums_proto_rawDesc = nil
	file_zorums_proto_goTypes = nil
	file_zorums_proto_depIdxs = nil
}
