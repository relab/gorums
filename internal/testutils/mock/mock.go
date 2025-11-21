package mock

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ServerMethodName is the method supported by the mock package.
const ServerMethodName = "mock.Server.Test"

var (
	// Mock Service Descriptors
	mockFile        *descriptorpb.FileDescriptorProto
	mockServiceDesc protoreflect.ServiceDescriptor
	mockMethodDesc  protoreflect.MethodDescriptor
	requestMsgDesc  protoreflect.MessageDescriptor
	responseMsgDesc protoreflect.MessageDescriptor
	requestType     protoreflect.MessageType
	responseType    protoreflect.MessageType
)

func init() {
	// Initialize Mock Definitions
	mockFile = &descriptorpb.FileDescriptorProto{
		Name:    proto.String("mock/mock.proto"),
		Package: proto.String("mock"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Request"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("val"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
			{
				Name: proto.String("Response"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("val"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("Server"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Test"),
						InputType:  proto.String(".mock.Request"),
						OutputType: proto.String(".mock.Response"),
					},
				},
			},
		},
	}
}

// Register registers the dynamic types in the global registry.
// It is safe to call multiple times.
func Register(t testing.TB) {
	t.Helper()
	if _, err := protoregistry.GlobalFiles.FindFileByPath(mockFile.GetName()); err == nil {
		// Already registered
		desc, _ := protoregistry.GlobalFiles.FindFileByPath(mockFile.GetName())
		mockServiceDesc = desc.Services().ByName("Server")
		mockMethodDesc = mockServiceDesc.Methods().ByName("Test")
		requestMsgDesc = desc.Messages().ByName("Request")
		responseMsgDesc = desc.Messages().ByName("Response")
		if requestMsgDesc == nil {
			t.Fatal("Request message not found in existing file")
		}
		if responseMsgDesc == nil {
			t.Fatal("Response message not found in existing file")
		}
		requestType = dynamicpb.NewMessageType(requestMsgDesc)
		responseType = dynamicpb.NewMessageType(responseMsgDesc)
		return
	}

	fd, err := protodesc.NewFile(mockFile, nil)
	if err != nil {
		t.Fatalf("failed to create mock file descriptor: %v", err)
	}

	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		t.Fatalf("failed to register mock file: %v", err)
	}

	mockServiceDesc = fd.Services().ByName("Server")
	mockMethodDesc = mockServiceDesc.Methods().ByName("Test")
	requestMsgDesc = fd.Messages().ByName("Request")
	responseMsgDesc = fd.Messages().ByName("Response")
	if requestMsgDesc == nil {
		t.Fatal("Request message not found in new file")
	}
	if responseMsgDesc == nil {
		t.Fatal("Response message not found in new file")
	}
	requestType = dynamicpb.NewMessageType(requestMsgDesc)
	responseType = dynamicpb.NewMessageType(responseMsgDesc)

	if err := protoregistry.GlobalTypes.RegisterMessage(requestType); err != nil {
		t.Fatalf("failed to register Request type: %v", err)
	}
	if err := protoregistry.GlobalTypes.RegisterMessage(responseType); err != nil {
		t.Fatalf("failed to register Response type: %v", err)
	}
}

// Helpers for Mock messages

func NewRequest(val string) proto.Message {
	msg := requestType.New()
	if val != "" {
		fd := msg.Descriptor().Fields().ByName("val")
		if fd != nil {
			msg.Set(fd, protoreflect.ValueOfString(val))
		}
	}
	return msg.Interface()
}

func NewResponse(val string) proto.Message {
	msg := responseType.New()
	if val != "" {
		fd := msg.Descriptor().Fields().ByName("val")
		if fd != nil {
			msg.Set(fd, protoreflect.ValueOfString(val))
		}
	}
	return msg.Interface()
}

func GetVal(msg proto.Message) string {
	m := msg.ProtoReflect()
	if !m.IsValid() {
		return ""
	}
	fd := m.Descriptor().Fields().ByName("val")
	if fd == nil {
		return ""
	}
	return m.Get(fd).String()
}

func SetVal(msg proto.Message, val string) {
	m := msg.ProtoReflect()
	if !m.IsValid() {
		return
	}
	fd := m.Descriptor().Fields().ByName("val")
	if fd == nil {
		return
	}
	m.Set(fd, protoreflect.ValueOfString(val))
}
