package impl

import (
	"fmt"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// UnmarshalRequest unmarshals the request proto message from the message.
// It uses the method name in the message to look up the Input type from the proto registry.
func UnmarshalRequest(in *stream.Message) (proto.Message, error) {
	return unmarshal(in, protoreflect.MethodDescriptor.Input, "request")
}

// unmarshalResponse unmarshals the response proto message from the message.
// It uses the method name in the message to look up the Output type from the proto registry.
func unmarshalResponse(out *stream.Message) (proto.Message, error) {
	return unmarshal(out, protoreflect.MethodDescriptor.Output, "response")
}

// unmarshal unmarshals the payload of msg into a new message of the type that
// msgDesc selects from the descriptor of the method msg names. The method
// name comes from the peer, so it is checked to name a registered method.
// kind names the message in errors.
func unmarshal(msg *stream.Message, msgDesc func(protoreflect.MethodDescriptor) protoreflect.MessageDescriptor, kind string) (proto.Message, error) {
	method := protoreflect.FullName(msg.GetMethod())
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(method)
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s: %w", method, err)
	}
	methodDesc, ok := desc.(protoreflect.MethodDescriptor)
	if !ok {
		return nil, fmt.Errorf("gorums: %s is not a method", method)
	}
	name := msgDesc(methodDesc).FullName()
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s: %w", name, err)
	}
	m := msgType.New().Interface()
	if payload := msg.GetPayload(); len(payload) > 0 {
		if err := proto.Unmarshal(payload, m); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal %s: %w", kind, err)
		}
	}
	return m, nil
}
