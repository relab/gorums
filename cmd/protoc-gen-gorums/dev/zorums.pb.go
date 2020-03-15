// Code generated by protoc-gen-go. DO NOT EDIT.
// source: zorums.proto

package dev

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	_ "github.com/relab/gorums"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type ReadRequest struct {
	Value                string   `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ReadRequest) Reset()         { *m = ReadRequest{} }
func (m *ReadRequest) String() string { return proto.CompactTextString(m) }
func (*ReadRequest) ProtoMessage()    {}
func (*ReadRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_eefcc80370fc6ec7, []int{0}
}

func (m *ReadRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ReadRequest.Unmarshal(m, b)
}
func (m *ReadRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ReadRequest.Marshal(b, m, deterministic)
}
func (m *ReadRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ReadRequest.Merge(m, src)
}
func (m *ReadRequest) XXX_Size() int {
	return xxx_messageInfo_ReadRequest.Size(m)
}
func (m *ReadRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ReadRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ReadRequest proto.InternalMessageInfo

func (m *ReadRequest) GetValue() string {
	if m != nil {
		return m.Value
	}
	return ""
}

type ReadResponse struct {
	Result               int64    `protobuf:"varint,1,opt,name=Result,proto3" json:"Result,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ReadResponse) Reset()         { *m = ReadResponse{} }
func (m *ReadResponse) String() string { return proto.CompactTextString(m) }
func (*ReadResponse) ProtoMessage()    {}
func (*ReadResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_eefcc80370fc6ec7, []int{1}
}

func (m *ReadResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ReadResponse.Unmarshal(m, b)
}
func (m *ReadResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ReadResponse.Marshal(b, m, deterministic)
}
func (m *ReadResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ReadResponse.Merge(m, src)
}
func (m *ReadResponse) XXX_Size() int {
	return xxx_messageInfo_ReadResponse.Size(m)
}
func (m *ReadResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ReadResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ReadResponse proto.InternalMessageInfo

func (m *ReadResponse) GetResult() int64 {
	if m != nil {
		return m.Result
	}
	return 0
}

type MyReadResponse struct {
	Value                string   `protobuf:"bytes,1,opt,name=Value,proto3" json:"Value,omitempty"`
	Result               int64    `protobuf:"varint,2,opt,name=Result,proto3" json:"Result,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MyReadResponse) Reset()         { *m = MyReadResponse{} }
func (m *MyReadResponse) String() string { return proto.CompactTextString(m) }
func (*MyReadResponse) ProtoMessage()    {}
func (*MyReadResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_eefcc80370fc6ec7, []int{2}
}

func (m *MyReadResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MyReadResponse.Unmarshal(m, b)
}
func (m *MyReadResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MyReadResponse.Marshal(b, m, deterministic)
}
func (m *MyReadResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MyReadResponse.Merge(m, src)
}
func (m *MyReadResponse) XXX_Size() int {
	return xxx_messageInfo_MyReadResponse.Size(m)
}
func (m *MyReadResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_MyReadResponse.DiscardUnknown(m)
}

var xxx_messageInfo_MyReadResponse proto.InternalMessageInfo

func (m *MyReadResponse) GetValue() string {
	if m != nil {
		return m.Value
	}
	return ""
}

func (m *MyReadResponse) GetResult() int64 {
	if m != nil {
		return m.Result
	}
	return 0
}

func init() {
	proto.RegisterType((*ReadRequest)(nil), "dev.ReadRequest")
	proto.RegisterType((*ReadResponse)(nil), "dev.ReadResponse")
	proto.RegisterType((*MyReadResponse)(nil), "dev.MyReadResponse")
}

func init() {
	proto.RegisterFile("zorums.proto", fileDescriptor_eefcc80370fc6ec7)
}

var fileDescriptor_eefcc80370fc6ec7 = []byte{
	// 390 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x93, 0x4d, 0x4f, 0xfa, 0x40,
	0x10, 0xc6, 0xff, 0xfd, 0xf3, 0x12, 0x5c, 0x11, 0x75, 0x45, 0x52, 0x31, 0x21, 0xa6, 0x26, 0x86,
	0x0b, 0xe0, 0xcb, 0x51, 0x63, 0x82, 0x18, 0x8c, 0x07, 0x50, 0x8a, 0xd1, 0x84, 0xc4, 0x43, 0x69,
	0x27, 0x48, 0xd2, 0xb2, 0x75, 0xbb, 0xdb, 0x04, 0x4f, 0x1e, 0x3d, 0x7a, 0xf4, 0xe8, 0xd1, 0x23,
	0x17, 0xbe, 0x80, 0xdf, 0xc7, 0x93, 0x5f, 0xc0, 0x2c, 0xc5, 0xa4, 0x6d, 0x38, 0xac, 0xc7, 0x7d,
	0x32, 0xcf, 0x6f, 0x9f, 0x99, 0xc9, 0xa0, 0xec, 0x13, 0xa1, 0xdc, 0xf1, 0xaa, 0x2e, 0x25, 0x8c,
	0xe0, 0x84, 0x05, 0x7e, 0x31, 0x3b, 0x08, 0x49, 0xda, 0x2e, 0x5a, 0xd6, 0xc1, 0xb0, 0x74, 0x78,
	0xe4, 0xe0, 0x31, 0x9c, 0x47, 0xa9, 0x5b, 0xc3, 0xe6, 0xa0, 0x2a, 0x3b, 0x4a, 0x79, 0x49, 0x0f,
	0x1e, 0xda, 0x1e, 0xca, 0x06, 0x45, 0x9e, 0x4b, 0x46, 0x1e, 0xe0, 0x02, 0x4a, 0xeb, 0xe0, 0x71,
	0x9b, 0xcd, 0xca, 0x12, 0xfa, 0xfc, 0xa5, 0x9d, 0xa2, 0x5c, 0x6b, 0x1c, 0xa9, 0x5c, 0xc8, 0x0b,
	0xf9, 0xff, 0x87, 0xfd, 0x87, 0x5f, 0x29, 0xb4, 0x22, 0xec, 0x40, 0xbb, 0x40, 0xfd, 0xa1, 0x09,
	0xf8, 0x00, 0x65, 0x84, 0x70, 0x41, 0x5d, 0x13, 0xaf, 0x55, 0x2d, 0xf0, 0xab, 0xa1, 0xb4, 0xc5,
	0xf5, 0x90, 0x12, 0x7c, 0xa8, 0xfd, 0xc3, 0xc7, 0x28, 0x27, 0x94, 0x0e, 0x17, 0x6d, 0x36, 0x0c,
	0xdb, 0x96, 0x33, 0x26, 0x9f, 0xa7, 0xaa, 0x82, 0x2f, 0x91, 0x1a, 0x35, 0x5f, 0x03, 0x6d, 0x13,
	0x0b, 0xea, 0x74, 0x20, 0x87, 0xc9, 0x08, 0xcc, 0x44, 0xa0, 0xae, 0x50, 0x29, 0x8a, 0xea, 0x34,
	0xef, 0x86, 0xec, 0x61, 0xee, 0xfd, 0x1b, 0xf0, 0x43, 0x00, 0xef, 0xe3, 0xc0, 0x06, 0xf7, 0x18,
	0x71, 0x74, 0x60, 0x9c, 0x8e, 0x6e, 0xc6, 0x2e, 0xc8, 0x01, 0x0b, 0x02, 0xf8, 0xf9, 0xad, 0xc6,
	0x57, 0xd5, 0x43, 0x1b, 0x31, 0x3c, 0x71, 0xfa, 0x44, 0x8e, 0x59, 0xfa, 0x0d, 0x39, 0x59, 0xcc,
	0x3e, 0x09, 0xf6, 0xda, 0xe2, 0x36, 0x1b, 0x9a, 0x86, 0xc7, 0x24, 0x57, 0xf2, 0x36, 0x55, 0x95,
	0xb2, 0x82, 0xeb, 0x28, 0x1f, 0x4d, 0xd6, 0xe4, 0x8c, 0x53, 0xc9, 0x76, 0x93, 0xef, 0x62, 0x76,
	0x6d, 0xb4, 0x2a, 0xd4, 0x06, 0xa1, 0x14, 0x4c, 0x66, 0xf4, 0x6d, 0xd9, 0x61, 0xbd, 0x2c, 0x6e,
	0xe8, 0x1c, 0x6d, 0xc6, 0x78, 0x5d, 0x46, 0xc1, 0x70, 0x24, 0x33, 0xbd, 0x4e, 0x55, 0x65, 0x5f,
	0x39, 0xdb, 0xee, 0x6d, 0x99, 0x8e, 0x55, 0x9b, 0x5d, 0xa2, 0x59, 0x19, 0xc0, 0xa8, 0x12, 0xdc,
	0x66, 0xcd, 0x02, 0xbf, 0x9f, 0x9e, 0xc9, 0x47, 0x3f, 0x01, 0x00, 0x00, 0xff, 0xff, 0xf2, 0x57,
	0xa5, 0xc5, 0xc3, 0x03, 0x00, 0x00,
}