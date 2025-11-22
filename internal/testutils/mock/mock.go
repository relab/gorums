package mock

import (
	"errors"
	"fmt"
	"sync"
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
	mockFile     *descriptorpb.FileDescriptorProto
	requestType  protoreflect.MessageType
	responseType protoreflect.MessageType
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

// Register registers the mock types in the global registry.
// It is safe to call multiple times.
func Register(t testing.TB) {
	t.Helper()
	if err := registerOnce(); err != nil {
		t.Fatal(err)
	}
}

var registerOnce = sync.OnceValue(func() error {
	if fd, err := protoregistry.GlobalFiles.FindFileByPath(mockFile.GetName()); err == nil {
		// Already registered
		return initDescriptors(fd)
	}

	fd, err := protodesc.NewFile(mockFile, nil)
	if err != nil {
		return fmt.Errorf("failed to create mock file descriptor: %v", err)
	}
	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		return fmt.Errorf("failed to register mock file: %v", err)
	}
	if err := initDescriptors(fd); err != nil {
		return fmt.Errorf("failed to initialize mock descriptors: %v", err)
	}
	if err := protoregistry.GlobalTypes.RegisterMessage(requestType); err != nil {
		return fmt.Errorf("failed to register Request type: %v", err)
	}
	if err := protoregistry.GlobalTypes.RegisterMessage(responseType); err != nil {
		return fmt.Errorf("failed to register Response type: %v", err)
	}
	return nil
})

func initDescriptors(fd protoreflect.FileDescriptor) error {
	mockServiceDesc := fd.Services().ByName("Server")
	if mockServiceDesc == nil {
		return errors.New("service Server not found")
	}
	mockMethodDesc := mockServiceDesc.Methods().ByName("Test")
	if mockMethodDesc == nil {
		return errors.New("method Test not found")
	}
	requestMsgDesc := fd.Messages().ByName("Request")
	if requestMsgDesc == nil {
		return errors.New("message Request not found")
	}
	responseMsgDesc := fd.Messages().ByName("Response")
	if responseMsgDesc == nil {
		return errors.New("message Response not found")
	}
	requestType = dynamicpb.NewMessageType(requestMsgDesc)
	responseType = dynamicpb.NewMessageType(responseMsgDesc)
	return nil
}

// Helpers for Mock messages

const panicMsg = "mock.Register() must be called before using NewRequest/NewResponse"

func NewRequest(val string) proto.Message {
	if requestType == nil {
		panic(panicMsg)
	}
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
	if responseType == nil {
		panic(panicMsg)
	}
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
	if msg == nil {
		return ""
	}
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
	if msg == nil {
		return
	}
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
