// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v4.25.3
// source: benchmark/benchmark.proto

package benchmark

import (
	_ "github.com/relab/gorums"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Echo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Payload []byte `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *Echo) Reset() {
	*x = Echo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Echo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Echo) ProtoMessage() {}

func (x *Echo) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Echo.ProtoReflect.Descriptor instead.
func (*Echo) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{0}
}

func (x *Echo) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

type TimedMsg struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SendTime int64  `protobuf:"varint,1,opt,name=SendTime,proto3" json:"SendTime,omitempty"`
	Payload  []byte `protobuf:"bytes,2,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *TimedMsg) Reset() {
	*x = TimedMsg{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TimedMsg) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TimedMsg) ProtoMessage() {}

func (x *TimedMsg) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TimedMsg.ProtoReflect.Descriptor instead.
func (*TimedMsg) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{1}
}

func (x *TimedMsg) GetSendTime() int64 {
	if x != nil {
		return x.SendTime
	}
	return 0
}

func (x *TimedMsg) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

type StartRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StartRequest) Reset() {
	*x = StartRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StartRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StartRequest) ProtoMessage() {}

func (x *StartRequest) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StartRequest.ProtoReflect.Descriptor instead.
func (*StartRequest) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{2}
}

type StartResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StartResponse) Reset() {
	*x = StartResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StartResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StartResponse) ProtoMessage() {}

func (x *StartResponse) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StartResponse.ProtoReflect.Descriptor instead.
func (*StartResponse) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{3}
}

type StopRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StopRequest) Reset() {
	*x = StopRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StopRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StopRequest) ProtoMessage() {}

func (x *StopRequest) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StopRequest.ProtoReflect.Descriptor instead.
func (*StopRequest) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{4}
}

type Result struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name        string        `protobuf:"bytes,1,opt,name=Name,proto3" json:"Name,omitempty"`
	TotalOps    uint64        `protobuf:"varint,2,opt,name=TotalOps,proto3" json:"TotalOps,omitempty"`
	TotalTime   int64         `protobuf:"varint,3,opt,name=TotalTime,proto3" json:"TotalTime,omitempty"`
	Throughput  float64       `protobuf:"fixed64,4,opt,name=Throughput,proto3" json:"Throughput,omitempty"`
	LatencyAvg  float64       `protobuf:"fixed64,5,opt,name=LatencyAvg,proto3" json:"LatencyAvg,omitempty"`
	LatencyVar  float64       `protobuf:"fixed64,6,opt,name=LatencyVar,proto3" json:"LatencyVar,omitempty"`
	AllocsPerOp uint64        `protobuf:"varint,7,opt,name=AllocsPerOp,proto3" json:"AllocsPerOp,omitempty"`
	MemPerOp    uint64        `protobuf:"varint,8,opt,name=MemPerOp,proto3" json:"MemPerOp,omitempty"`
	ServerStats []*MemoryStat `protobuf:"bytes,9,rep,name=ServerStats,proto3" json:"ServerStats,omitempty"`
}

func (x *Result) Reset() {
	*x = Result{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Result) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Result) ProtoMessage() {}

func (x *Result) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Result.ProtoReflect.Descriptor instead.
func (*Result) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{5}
}

func (x *Result) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Result) GetTotalOps() uint64 {
	if x != nil {
		return x.TotalOps
	}
	return 0
}

func (x *Result) GetTotalTime() int64 {
	if x != nil {
		return x.TotalTime
	}
	return 0
}

func (x *Result) GetThroughput() float64 {
	if x != nil {
		return x.Throughput
	}
	return 0
}

func (x *Result) GetLatencyAvg() float64 {
	if x != nil {
		return x.LatencyAvg
	}
	return 0
}

func (x *Result) GetLatencyVar() float64 {
	if x != nil {
		return x.LatencyVar
	}
	return 0
}

func (x *Result) GetAllocsPerOp() uint64 {
	if x != nil {
		return x.AllocsPerOp
	}
	return 0
}

func (x *Result) GetMemPerOp() uint64 {
	if x != nil {
		return x.MemPerOp
	}
	return 0
}

func (x *Result) GetServerStats() []*MemoryStat {
	if x != nil {
		return x.ServerStats
	}
	return nil
}

type MemoryStat struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Allocs uint64 `protobuf:"varint,1,opt,name=Allocs,proto3" json:"Allocs,omitempty"`
	Memory uint64 `protobuf:"varint,2,opt,name=Memory,proto3" json:"Memory,omitempty"`
}

func (x *MemoryStat) Reset() {
	*x = MemoryStat{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MemoryStat) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MemoryStat) ProtoMessage() {}

func (x *MemoryStat) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MemoryStat.ProtoReflect.Descriptor instead.
func (*MemoryStat) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{6}
}

func (x *MemoryStat) GetAllocs() uint64 {
	if x != nil {
		return x.Allocs
	}
	return 0
}

func (x *MemoryStat) GetMemory() uint64 {
	if x != nil {
		return x.Memory
	}
	return 0
}

type MemoryStatList struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	MemoryStats []*MemoryStat `protobuf:"bytes,1,rep,name=MemoryStats,proto3" json:"MemoryStats,omitempty"`
}

func (x *MemoryStatList) Reset() {
	*x = MemoryStatList{}
	if protoimpl.UnsafeEnabled {
		mi := &file_benchmark_benchmark_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MemoryStatList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MemoryStatList) ProtoMessage() {}

func (x *MemoryStatList) ProtoReflect() protoreflect.Message {
	mi := &file_benchmark_benchmark_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MemoryStatList.ProtoReflect.Descriptor instead.
func (*MemoryStatList) Descriptor() ([]byte, []int) {
	return file_benchmark_benchmark_proto_rawDescGZIP(), []int{7}
}

func (x *MemoryStatList) GetMemoryStats() []*MemoryStat {
	if x != nil {
		return x.MemoryStats
	}
	return nil
}

var File_benchmark_benchmark_proto protoreflect.FileDescriptor

var file_benchmark_benchmark_proto_rawDesc = []byte{
	0x0a, 0x19, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2f, 0x62, 0x65, 0x6e, 0x63,
	0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x09, 0x62, 0x65, 0x6e,
	0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x0c, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0x20, 0x0a, 0x04, 0x45, 0x63, 0x68, 0x6f, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x61, 0x79,
	0x6c, 0x6f, 0x61, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x70, 0x61, 0x79, 0x6c,
	0x6f, 0x61, 0x64, 0x22, 0x40, 0x0a, 0x08, 0x54, 0x69, 0x6d, 0x65, 0x64, 0x4d, 0x73, 0x67, 0x12,
	0x1a, 0x0a, 0x08, 0x53, 0x65, 0x6e, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x03, 0x52, 0x08, 0x53, 0x65, 0x6e, 0x64, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x70,
	0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x70, 0x61,
	0x79, 0x6c, 0x6f, 0x61, 0x64, 0x22, 0x0e, 0x0a, 0x0c, 0x53, 0x74, 0x61, 0x72, 0x74, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x0f, 0x0a, 0x0d, 0x53, 0x74, 0x61, 0x72, 0x74, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x0d, 0x0a, 0x0b, 0x53, 0x74, 0x6f, 0x70, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0xad, 0x02, 0x0a, 0x06, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x12, 0x12, 0x0a, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x4e, 0x61, 0x6d, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x54, 0x6f, 0x74, 0x61, 0x6c, 0x4f, 0x70, 0x73,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x08, 0x54, 0x6f, 0x74, 0x61, 0x6c, 0x4f, 0x70, 0x73,
	0x12, 0x1c, 0x0a, 0x09, 0x54, 0x6f, 0x74, 0x61, 0x6c, 0x54, 0x69, 0x6d, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x09, 0x54, 0x6f, 0x74, 0x61, 0x6c, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x1e,
	0x0a, 0x0a, 0x54, 0x68, 0x72, 0x6f, 0x75, 0x67, 0x68, 0x70, 0x75, 0x74, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x01, 0x52, 0x0a, 0x54, 0x68, 0x72, 0x6f, 0x75, 0x67, 0x68, 0x70, 0x75, 0x74, 0x12, 0x1e,
	0x0a, 0x0a, 0x4c, 0x61, 0x74, 0x65, 0x6e, 0x63, 0x79, 0x41, 0x76, 0x67, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x01, 0x52, 0x0a, 0x4c, 0x61, 0x74, 0x65, 0x6e, 0x63, 0x79, 0x41, 0x76, 0x67, 0x12, 0x1e,
	0x0a, 0x0a, 0x4c, 0x61, 0x74, 0x65, 0x6e, 0x63, 0x79, 0x56, 0x61, 0x72, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x01, 0x52, 0x0a, 0x4c, 0x61, 0x74, 0x65, 0x6e, 0x63, 0x79, 0x56, 0x61, 0x72, 0x12, 0x20,
	0x0a, 0x0b, 0x41, 0x6c, 0x6c, 0x6f, 0x63, 0x73, 0x50, 0x65, 0x72, 0x4f, 0x70, 0x18, 0x07, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x0b, 0x41, 0x6c, 0x6c, 0x6f, 0x63, 0x73, 0x50, 0x65, 0x72, 0x4f, 0x70,
	0x12, 0x1a, 0x0a, 0x08, 0x4d, 0x65, 0x6d, 0x50, 0x65, 0x72, 0x4f, 0x70, 0x18, 0x08, 0x20, 0x01,
	0x28, 0x04, 0x52, 0x08, 0x4d, 0x65, 0x6d, 0x50, 0x65, 0x72, 0x4f, 0x70, 0x12, 0x37, 0x0a, 0x0b,
	0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x73, 0x18, 0x09, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x15, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x4d, 0x65,
	0x6d, 0x6f, 0x72, 0x79, 0x53, 0x74, 0x61, 0x74, 0x52, 0x0b, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x53, 0x74, 0x61, 0x74, 0x73, 0x22, 0x3c, 0x0a, 0x0a, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53,
	0x74, 0x61, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x41, 0x6c, 0x6c, 0x6f, 0x63, 0x73, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x06, 0x41, 0x6c, 0x6c, 0x6f, 0x63, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x4d,
	0x65, 0x6d, 0x6f, 0x72, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x06, 0x4d, 0x65, 0x6d,
	0x6f, 0x72, 0x79, 0x22, 0x49, 0x0a, 0x0e, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x74, 0x61,
	0x74, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x37, 0x0a, 0x0b, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53,
	0x74, 0x61, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x62, 0x65, 0x6e,
	0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x74, 0x61,
	0x74, 0x52, 0x0b, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x74, 0x61, 0x74, 0x73, 0x32, 0xb2,
	0x04, 0x0a, 0x09, 0x42, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x12, 0x4f, 0x0a, 0x14,
	0x53, 0x74, 0x61, 0x72, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x42, 0x65, 0x6e, 0x63, 0x68,
	0x6d, 0x61, 0x72, 0x6b, 0x12, 0x17, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b,
	0x2e, 0x53, 0x74, 0x61, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e,
	0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x53, 0x74, 0x61, 0x72, 0x74, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x46, 0x0a,
	0x13, 0x53, 0x74, 0x6f, 0x70, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x42, 0x65, 0x6e, 0x63, 0x68,
	0x6d, 0x61, 0x72, 0x6b, 0x12, 0x16, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b,
	0x2e, 0x53, 0x74, 0x6f, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x11, 0x2e, 0x62,
	0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x22,
	0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x49, 0x0a, 0x0e, 0x53, 0x74, 0x61, 0x72, 0x74, 0x42, 0x65,
	0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x12, 0x17, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d,
	0x61, 0x72, 0x6b, 0x2e, 0x53, 0x74, 0x61, 0x72, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x18, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x53, 0x74, 0x61,
	0x72, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01,
	0x12, 0x56, 0x0a, 0x0d, 0x53, 0x74, 0x6f, 0x70, 0x42, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72,
	0x6b, 0x12, 0x16, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x53, 0x74,
	0x6f, 0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x15, 0x2e, 0x62, 0x65, 0x6e, 0x63,
	0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x53, 0x74, 0x61, 0x74,
	0x22, 0x16, 0xa0, 0xb5, 0x18, 0x01, 0xf2, 0xb6, 0x18, 0x0e, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79,
	0x53, 0x74, 0x61, 0x74, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x34, 0x0a, 0x0a, 0x51, 0x75, 0x6f, 0x72,
	0x75, 0x6d, 0x43, 0x61, 0x6c, 0x6c, 0x12, 0x0f, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61,
	0x72, 0x6b, 0x2e, 0x45, 0x63, 0x68, 0x6f, 0x1a, 0x0f, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d,
	0x61, 0x72, 0x6b, 0x2e, 0x45, 0x63, 0x68, 0x6f, 0x22, 0x04, 0xa0, 0xb5, 0x18, 0x01, 0x12, 0x3d,
	0x0a, 0x0f, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x61, 0x6c,
	0x6c, 0x12, 0x0f, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x45, 0x63,
	0x68, 0x6f, 0x1a, 0x0f, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x45,
	0x63, 0x68, 0x6f, 0x22, 0x08, 0xa0, 0xb5, 0x18, 0x01, 0xd0, 0xb5, 0x18, 0x01, 0x12, 0x34, 0x0a,
	0x0a, 0x53, 0x6c, 0x6f, 0x77, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x0f, 0x2e, 0x62, 0x65,
	0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x45, 0x63, 0x68, 0x6f, 0x1a, 0x0f, 0x2e, 0x62,
	0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x45, 0x63, 0x68, 0x6f, 0x22, 0x04, 0xa0,
	0xb5, 0x18, 0x01, 0x12, 0x3e, 0x0a, 0x09, 0x4d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x61, 0x73, 0x74,
	0x12, 0x13, 0x2e, 0x62, 0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x2e, 0x54, 0x69, 0x6d,
	0x65, 0x64, 0x4d, 0x73, 0x67, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x04, 0x98,
	0xb5, 0x18, 0x01, 0x42, 0x23, 0x5a, 0x21, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f,
	0x6d, 0x2f, 0x72, 0x65, 0x6c, 0x61, 0x62, 0x2f, 0x67, 0x6f, 0x72, 0x75, 0x6d, 0x73, 0x2f, 0x62,
	0x65, 0x6e, 0x63, 0x68, 0x6d, 0x61, 0x72, 0x6b, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_benchmark_benchmark_proto_rawDescOnce sync.Once
	file_benchmark_benchmark_proto_rawDescData = file_benchmark_benchmark_proto_rawDesc
)

func file_benchmark_benchmark_proto_rawDescGZIP() []byte {
	file_benchmark_benchmark_proto_rawDescOnce.Do(func() {
		file_benchmark_benchmark_proto_rawDescData = protoimpl.X.CompressGZIP(file_benchmark_benchmark_proto_rawDescData)
	})
	return file_benchmark_benchmark_proto_rawDescData
}

var file_benchmark_benchmark_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_benchmark_benchmark_proto_goTypes = []interface{}{
	(*Echo)(nil),           // 0: benchmark.Echo
	(*TimedMsg)(nil),       // 1: benchmark.TimedMsg
	(*StartRequest)(nil),   // 2: benchmark.StartRequest
	(*StartResponse)(nil),  // 3: benchmark.StartResponse
	(*StopRequest)(nil),    // 4: benchmark.StopRequest
	(*Result)(nil),         // 5: benchmark.Result
	(*MemoryStat)(nil),     // 6: benchmark.MemoryStat
	(*MemoryStatList)(nil), // 7: benchmark.MemoryStatList
	(*emptypb.Empty)(nil),  // 8: google.protobuf.Empty
}
var file_benchmark_benchmark_proto_depIdxs = []int32{
	6,  // 0: benchmark.Result.ServerStats:type_name -> benchmark.MemoryStat
	6,  // 1: benchmark.MemoryStatList.MemoryStats:type_name -> benchmark.MemoryStat
	2,  // 2: benchmark.Benchmark.StartServerBenchmark:input_type -> benchmark.StartRequest
	4,  // 3: benchmark.Benchmark.StopServerBenchmark:input_type -> benchmark.StopRequest
	2,  // 4: benchmark.Benchmark.StartBenchmark:input_type -> benchmark.StartRequest
	4,  // 5: benchmark.Benchmark.StopBenchmark:input_type -> benchmark.StopRequest
	0,  // 6: benchmark.Benchmark.QuorumCall:input_type -> benchmark.Echo
	0,  // 7: benchmark.Benchmark.AsyncQuorumCall:input_type -> benchmark.Echo
	0,  // 8: benchmark.Benchmark.SlowServer:input_type -> benchmark.Echo
	1,  // 9: benchmark.Benchmark.Multicast:input_type -> benchmark.TimedMsg
	3,  // 10: benchmark.Benchmark.StartServerBenchmark:output_type -> benchmark.StartResponse
	5,  // 11: benchmark.Benchmark.StopServerBenchmark:output_type -> benchmark.Result
	3,  // 12: benchmark.Benchmark.StartBenchmark:output_type -> benchmark.StartResponse
	6,  // 13: benchmark.Benchmark.StopBenchmark:output_type -> benchmark.MemoryStat
	0,  // 14: benchmark.Benchmark.QuorumCall:output_type -> benchmark.Echo
	0,  // 15: benchmark.Benchmark.AsyncQuorumCall:output_type -> benchmark.Echo
	0,  // 16: benchmark.Benchmark.SlowServer:output_type -> benchmark.Echo
	8,  // 17: benchmark.Benchmark.Multicast:output_type -> google.protobuf.Empty
	10, // [10:18] is the sub-list for method output_type
	2,  // [2:10] is the sub-list for method input_type
	2,  // [2:2] is the sub-list for extension type_name
	2,  // [2:2] is the sub-list for extension extendee
	0,  // [0:2] is the sub-list for field type_name
}

func init() { file_benchmark_benchmark_proto_init() }
func file_benchmark_benchmark_proto_init() {
	if File_benchmark_benchmark_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_benchmark_benchmark_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Echo); i {
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
		file_benchmark_benchmark_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TimedMsg); i {
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
		file_benchmark_benchmark_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StartRequest); i {
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
		file_benchmark_benchmark_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StartResponse); i {
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
		file_benchmark_benchmark_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StopRequest); i {
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
		file_benchmark_benchmark_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Result); i {
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
		file_benchmark_benchmark_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MemoryStat); i {
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
		file_benchmark_benchmark_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MemoryStatList); i {
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
			RawDescriptor: file_benchmark_benchmark_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_benchmark_benchmark_proto_goTypes,
		DependencyIndexes: file_benchmark_benchmark_proto_depIdxs,
		MessageInfos:      file_benchmark_benchmark_proto_msgTypes,
	}.Build()
	File_benchmark_benchmark_proto = out.File
	file_benchmark_benchmark_proto_rawDesc = nil
	file_benchmark_benchmark_proto_goTypes = nil
	file_benchmark_benchmark_proto_depIdxs = nil
}
