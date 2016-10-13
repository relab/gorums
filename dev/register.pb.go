// Code generated by protoc-gen-gogo.
// source: dev/register.proto
// DO NOT EDIT!

/*
Package dev is a generated protocol buffer package.

It is generated from these files:
	dev/register.proto

It has these top-level messages:
	State
	WriteResponse
	ReadRequest
	Empty
*/
package dev

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "github.com/relab/gorums"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

import strings "strings"
import reflect "reflect"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type State struct {
	Value     string `protobuf:"bytes,1,opt,name=Value,json=value,proto3" json:"Value,omitempty"`
	Timestamp int64  `protobuf:"varint,2,opt,name=Timestamp,json=timestamp,proto3" json:"Timestamp,omitempty"`
}

func (m *State) Reset()                    { *m = State{} }
func (*State) ProtoMessage()               {}
func (*State) Descriptor() ([]byte, []int) { return fileDescriptorRegister, []int{0} }

type WriteResponse struct {
	New bool `protobuf:"varint,1,opt,name=New,json=new,proto3" json:"New,omitempty"`
}

func (m *WriteResponse) Reset()                    { *m = WriteResponse{} }
func (*WriteResponse) ProtoMessage()               {}
func (*WriteResponse) Descriptor() ([]byte, []int) { return fileDescriptorRegister, []int{1} }

type ReadRequest struct {
}

func (m *ReadRequest) Reset()                    { *m = ReadRequest{} }
func (*ReadRequest) ProtoMessage()               {}
func (*ReadRequest) Descriptor() ([]byte, []int) { return fileDescriptorRegister, []int{2} }

type Empty struct {
}

func (m *Empty) Reset()                    { *m = Empty{} }
func (*Empty) ProtoMessage()               {}
func (*Empty) Descriptor() ([]byte, []int) { return fileDescriptorRegister, []int{3} }

func init() {
	proto.RegisterType((*State)(nil), "dev.State")
	proto.RegisterType((*WriteResponse)(nil), "dev.WriteResponse")
	proto.RegisterType((*ReadRequest)(nil), "dev.ReadRequest")
	proto.RegisterType((*Empty)(nil), "dev.Empty")
}
func (this *State) VerboseEqual(that interface{}) error {
	if that == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that == nil && this != nil")
	}

	that1, ok := that.(*State)
	if !ok {
		that2, ok := that.(State)
		if ok {
			that1 = &that2
		} else {
			return fmt.Errorf("that is not of type *State")
		}
	}
	if that1 == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that is type *State but is nil && this != nil")
	} else if this == nil {
		return fmt.Errorf("that is type *State but is not nil && this == nil")
	}
	if this.Value != that1.Value {
		return fmt.Errorf("Value this(%v) Not Equal that(%v)", this.Value, that1.Value)
	}
	if this.Timestamp != that1.Timestamp {
		return fmt.Errorf("Timestamp this(%v) Not Equal that(%v)", this.Timestamp, that1.Timestamp)
	}
	return nil
}
func (this *State) Equal(that interface{}) bool {
	if that == nil {
		if this == nil {
			return true
		}
		return false
	}

	that1, ok := that.(*State)
	if !ok {
		that2, ok := that.(State)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		if this == nil {
			return true
		}
		return false
	} else if this == nil {
		return false
	}
	if this.Value != that1.Value {
		return false
	}
	if this.Timestamp != that1.Timestamp {
		return false
	}
	return true
}
func (this *WriteResponse) VerboseEqual(that interface{}) error {
	if that == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that == nil && this != nil")
	}

	that1, ok := that.(*WriteResponse)
	if !ok {
		that2, ok := that.(WriteResponse)
		if ok {
			that1 = &that2
		} else {
			return fmt.Errorf("that is not of type *WriteResponse")
		}
	}
	if that1 == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that is type *WriteResponse but is nil && this != nil")
	} else if this == nil {
		return fmt.Errorf("that is type *WriteResponse but is not nil && this == nil")
	}
	if this.New != that1.New {
		return fmt.Errorf("New this(%v) Not Equal that(%v)", this.New, that1.New)
	}
	return nil
}
func (this *WriteResponse) Equal(that interface{}) bool {
	if that == nil {
		if this == nil {
			return true
		}
		return false
	}

	that1, ok := that.(*WriteResponse)
	if !ok {
		that2, ok := that.(WriteResponse)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		if this == nil {
			return true
		}
		return false
	} else if this == nil {
		return false
	}
	if this.New != that1.New {
		return false
	}
	return true
}
func (this *ReadRequest) VerboseEqual(that interface{}) error {
	if that == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that == nil && this != nil")
	}

	that1, ok := that.(*ReadRequest)
	if !ok {
		that2, ok := that.(ReadRequest)
		if ok {
			that1 = &that2
		} else {
			return fmt.Errorf("that is not of type *ReadRequest")
		}
	}
	if that1 == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that is type *ReadRequest but is nil && this != nil")
	} else if this == nil {
		return fmt.Errorf("that is type *ReadRequest but is not nil && this == nil")
	}
	return nil
}
func (this *ReadRequest) Equal(that interface{}) bool {
	if that == nil {
		if this == nil {
			return true
		}
		return false
	}

	that1, ok := that.(*ReadRequest)
	if !ok {
		that2, ok := that.(ReadRequest)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		if this == nil {
			return true
		}
		return false
	} else if this == nil {
		return false
	}
	return true
}
func (this *Empty) VerboseEqual(that interface{}) error {
	if that == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that == nil && this != nil")
	}

	that1, ok := that.(*Empty)
	if !ok {
		that2, ok := that.(Empty)
		if ok {
			that1 = &that2
		} else {
			return fmt.Errorf("that is not of type *Empty")
		}
	}
	if that1 == nil {
		if this == nil {
			return nil
		}
		return fmt.Errorf("that is type *Empty but is nil && this != nil")
	} else if this == nil {
		return fmt.Errorf("that is type *Empty but is not nil && this == nil")
	}
	return nil
}
func (this *Empty) Equal(that interface{}) bool {
	if that == nil {
		if this == nil {
			return true
		}
		return false
	}

	that1, ok := that.(*Empty)
	if !ok {
		that2, ok := that.(Empty)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		if this == nil {
			return true
		}
		return false
	} else if this == nil {
		return false
	}
	return true
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion3

// Client API for Register service

type RegisterClient interface {
	Read(ctx context.Context, in *ReadRequest, opts ...grpc.CallOption) (*State, error)
	Write(ctx context.Context, in *State, opts ...grpc.CallOption) (*WriteResponse, error)
	WriteAsync(ctx context.Context, opts ...grpc.CallOption) (Register_WriteAsyncClient, error)
	ReadNoQRPC(ctx context.Context, in *ReadRequest, opts ...grpc.CallOption) (*State, error)
}

type registerClient struct {
	cc *grpc.ClientConn
}

func NewRegisterClient(cc *grpc.ClientConn) RegisterClient {
	return &registerClient{cc}
}

func (c *registerClient) Read(ctx context.Context, in *ReadRequest, opts ...grpc.CallOption) (*State, error) {
	out := new(State)
	err := grpc.Invoke(ctx, "/dev.Register/Read", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *registerClient) Write(ctx context.Context, in *State, opts ...grpc.CallOption) (*WriteResponse, error) {
	out := new(WriteResponse)
	err := grpc.Invoke(ctx, "/dev.Register/Write", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *registerClient) WriteAsync(ctx context.Context, opts ...grpc.CallOption) (Register_WriteAsyncClient, error) {
	stream, err := grpc.NewClientStream(ctx, &_Register_serviceDesc.Streams[0], c.cc, "/dev.Register/WriteAsync", opts...)
	if err != nil {
		return nil, err
	}
	x := &registerWriteAsyncClient{stream}
	return x, nil
}

type Register_WriteAsyncClient interface {
	Send(*State) error
	CloseAndRecv() (*Empty, error)
	grpc.ClientStream
}

type registerWriteAsyncClient struct {
	grpc.ClientStream
}

func (x *registerWriteAsyncClient) Send(m *State) error {
	return x.ClientStream.SendMsg(m)
}

func (x *registerWriteAsyncClient) CloseAndRecv() (*Empty, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(Empty)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *registerClient) ReadNoQRPC(ctx context.Context, in *ReadRequest, opts ...grpc.CallOption) (*State, error) {
	out := new(State)
	err := grpc.Invoke(ctx, "/dev.Register/ReadNoQRPC", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for Register service

type RegisterServer interface {
	Read(context.Context, *ReadRequest) (*State, error)
	Write(context.Context, *State) (*WriteResponse, error)
	WriteAsync(Register_WriteAsyncServer) error
	ReadNoQRPC(context.Context, *ReadRequest) (*State, error)
}

func RegisterRegisterServer(s *grpc.Server, srv RegisterServer) {
	s.RegisterService(&_Register_serviceDesc, srv)
}

func _Register_Read_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RegisterServer).Read(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dev.Register/Read",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RegisterServer).Read(ctx, req.(*ReadRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Register_Write_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(State)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RegisterServer).Write(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dev.Register/Write",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RegisterServer).Write(ctx, req.(*State))
	}
	return interceptor(ctx, in, info, handler)
}

func _Register_WriteAsync_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RegisterServer).WriteAsync(&registerWriteAsyncServer{stream})
}

type Register_WriteAsyncServer interface {
	SendAndClose(*Empty) error
	Recv() (*State, error)
	grpc.ServerStream
}

type registerWriteAsyncServer struct {
	grpc.ServerStream
}

func (x *registerWriteAsyncServer) SendAndClose(m *Empty) error {
	return x.ServerStream.SendMsg(m)
}

func (x *registerWriteAsyncServer) Recv() (*State, error) {
	m := new(State)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Register_ReadNoQRPC_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RegisterServer).ReadNoQRPC(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/dev.Register/ReadNoQRPC",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RegisterServer).ReadNoQRPC(ctx, req.(*ReadRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Register_serviceDesc = grpc.ServiceDesc{
	ServiceName: "dev.Register",
	HandlerType: (*RegisterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Read",
			Handler:    _Register_Read_Handler,
		},
		{
			MethodName: "Write",
			Handler:    _Register_Write_Handler,
		},
		{
			MethodName: "ReadNoQRPC",
			Handler:    _Register_ReadNoQRPC_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "WriteAsync",
			Handler:       _Register_WriteAsync_Handler,
			ClientStreams: true,
		},
	},
	Metadata: fileDescriptorRegister,
}

func (m *State) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *State) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Value) > 0 {
		data[i] = 0xa
		i++
		i = encodeVarintRegister(data, i, uint64(len(m.Value)))
		i += copy(data[i:], m.Value)
	}
	if m.Timestamp != 0 {
		data[i] = 0x10
		i++
		i = encodeVarintRegister(data, i, uint64(m.Timestamp))
	}
	return i, nil
}

func (m *WriteResponse) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *WriteResponse) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.New {
		data[i] = 0x8
		i++
		if m.New {
			data[i] = 1
		} else {
			data[i] = 0
		}
		i++
	}
	return i, nil
}

func (m *ReadRequest) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *ReadRequest) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	return i, nil
}

func (m *Empty) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *Empty) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	return i, nil
}

func encodeFixed64Register(data []byte, offset int, v uint64) int {
	data[offset] = uint8(v)
	data[offset+1] = uint8(v >> 8)
	data[offset+2] = uint8(v >> 16)
	data[offset+3] = uint8(v >> 24)
	data[offset+4] = uint8(v >> 32)
	data[offset+5] = uint8(v >> 40)
	data[offset+6] = uint8(v >> 48)
	data[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Register(data []byte, offset int, v uint32) int {
	data[offset] = uint8(v)
	data[offset+1] = uint8(v >> 8)
	data[offset+2] = uint8(v >> 16)
	data[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintRegister(data []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		data[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	data[offset] = uint8(v)
	return offset + 1
}
func (m *State) Size() (n int) {
	var l int
	_ = l
	l = len(m.Value)
	if l > 0 {
		n += 1 + l + sovRegister(uint64(l))
	}
	if m.Timestamp != 0 {
		n += 1 + sovRegister(uint64(m.Timestamp))
	}
	return n
}

func (m *WriteResponse) Size() (n int) {
	var l int
	_ = l
	if m.New {
		n += 2
	}
	return n
}

func (m *ReadRequest) Size() (n int) {
	var l int
	_ = l
	return n
}

func (m *Empty) Size() (n int) {
	var l int
	_ = l
	return n
}

func sovRegister(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozRegister(x uint64) (n int) {
	return sovRegister(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (this *State) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&State{`,
		`Value:` + fmt.Sprintf("%v", this.Value) + `,`,
		`Timestamp:` + fmt.Sprintf("%v", this.Timestamp) + `,`,
		`}`,
	}, "")
	return s
}
func (this *WriteResponse) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&WriteResponse{`,
		`New:` + fmt.Sprintf("%v", this.New) + `,`,
		`}`,
	}, "")
	return s
}
func (this *ReadRequest) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&ReadRequest{`,
		`}`,
	}, "")
	return s
}
func (this *Empty) String() string {
	if this == nil {
		return "nil"
	}
	s := strings.Join([]string{`&Empty{`,
		`}`,
	}, "")
	return s
}
func valueToStringRegister(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.IsNil() {
		return "nil"
	}
	pv := reflect.Indirect(rv).Interface()
	return fmt.Sprintf("*%v", pv)
}
func (m *State) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRegister
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: State: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: State: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRegister
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthRegister
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Value = string(data[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Timestamp", wireType)
			}
			m.Timestamp = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRegister
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				m.Timestamp |= (int64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipRegister(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRegister
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *WriteResponse) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRegister
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: WriteResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: WriteResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field New", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowRegister
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				v |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.New = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipRegister(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRegister
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *ReadRequest) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRegister
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: ReadRequest: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: ReadRequest: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipRegister(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRegister
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Empty) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowRegister
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Empty: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Empty: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipRegister(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthRegister
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipRegister(data []byte) (n int, err error) {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowRegister
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowRegister
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if data[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowRegister
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthRegister
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowRegister
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := data[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipRegister(data[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthRegister = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowRegister   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("dev/register.proto", fileDescriptorRegister) }

var fileDescriptorRegister = []byte{
	// 326 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x84, 0x90, 0x31, 0x4f, 0xf2, 0x40,
	0x18, 0xc7, 0xfb, 0xbc, 0xa5, 0xaf, 0xf0, 0x18, 0x12, 0x72, 0x71, 0x68, 0x88, 0xb9, 0x60, 0xe3,
	0x40, 0x0c, 0x96, 0x44, 0x47, 0x27, 0x35, 0xae, 0x44, 0x4f, 0xa3, 0x73, 0xa1, 0x4f, 0xb0, 0x09,
	0xa5, 0xb5, 0x77, 0x2d, 0x61, 0x63, 0x74, 0xf4, 0x63, 0xf8, 0x05, 0xf8, 0x00, 0x6e, 0x8e, 0x8c,
	0x8e, 0x70, 0x2e, 0x8e, 0x7e, 0x04, 0xc3, 0x81, 0x09, 0x4c, 0x4e, 0xf7, 0xdc, 0xff, 0x7e, 0x77,
	0xff, 0x5f, 0x0e, 0x59, 0x48, 0x45, 0x3b, 0xa3, 0x7e, 0x24, 0x15, 0x65, 0x7e, 0x9a, 0x25, 0x2a,
	0x61, 0x76, 0x48, 0x45, 0xfd, 0xb0, 0x1f, 0xa9, 0xc7, 0xbc, 0xeb, 0xf7, 0x92, 0xb8, 0x9d, 0xd1,
	0x20, 0xe8, 0xb6, 0xfb, 0x49, 0x96, 0xc7, 0x72, 0xbd, 0xac, 0x50, 0xef, 0x0c, 0x9d, 0x5b, 0x15,
	0x28, 0x62, 0x7b, 0xe8, 0xdc, 0x07, 0x83, 0x9c, 0x5c, 0x68, 0x40, 0xb3, 0x22, 0x9c, 0x62, 0xb9,
	0x61, 0xfb, 0x58, 0xb9, 0x8b, 0x62, 0x92, 0x2a, 0x88, 0x53, 0xf7, 0x5f, 0x03, 0x9a, 0xb6, 0xa8,
	0xa8, 0xdf, 0xc0, 0x3b, 0xc0, 0xea, 0x43, 0x16, 0x29, 0x12, 0x24, 0xd3, 0x64, 0x28, 0x89, 0xd5,
	0xd0, 0xee, 0xd0, 0xc8, 0x3c, 0x51, 0x16, 0xf6, 0x90, 0x46, 0x5e, 0x15, 0x77, 0x05, 0x05, 0xa1,
	0xa0, 0xa7, 0x9c, 0xa4, 0xf2, 0x76, 0xd0, 0xb9, 0x8a, 0x53, 0x35, 0x3e, 0x79, 0x03, 0x2c, 0x8b,
	0xb5, 0x35, 0x3b, 0xc2, 0xd2, 0x12, 0x62, 0x35, 0x3f, 0xa4, 0xc2, 0xdf, 0xe0, 0xeb, 0x68, 0x12,
	0x63, 0xe8, 0x95, 0x26, 0x53, 0x17, 0xd8, 0x31, 0x3a, 0xa6, 0x93, 0x6d, 0x1c, 0xd5, 0x99, 0x99,
	0xb7, 0x5c, 0xd6, 0x78, 0x0b, 0xd1, 0xc4, 0xe7, 0x72, 0x3c, 0xec, 0x6d, 0xdd, 0x59, 0xcd, 0xc6,
	0xc6, 0x2b, 0x3d, 0x4f, 0x5d, 0x68, 0x1a, 0x7a, 0xd9, 0xde, 0x49, 0x6e, 0xc4, 0xf5, 0xe5, 0x1f,
	0x3a, 0xd6, 0x45, 0x6b, 0xb6, 0xe0, 0xd6, 0xc7, 0x82, 0x5b, 0xf3, 0x05, 0x87, 0x89, 0xe6, 0xf0,
	0xaa, 0x39, 0xbc, 0x6b, 0x0e, 0x33, 0xcd, 0x61, 0xae, 0x39, 0x7c, 0x69, 0x6e, 0x7d, 0x6b, 0x0e,
	0x2f, 0x9f, 0xdc, 0xea, 0xfe, 0x37, 0x1f, 0x7e, 0xfa, 0x13, 0x00, 0x00, 0xff, 0xff, 0xb4, 0xbe,
	0x2c, 0x6d, 0xb1, 0x01, 0x00, 0x00,
}
