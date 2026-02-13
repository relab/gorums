package stream

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// UnmarshalRequest unmarshals the request proto message from the message.
// It uses the method name in the message to look up the Input type from the proto registry.
//
// This function should only be used by internal channel operations.
func UnmarshalRequest(in *Message) (proto.Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(in.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", in.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the request message type (Input type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Input().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Input().FullName())
	}
	req := msgType.New().Interface()

	// unmarshal message from the Message.Payload field
	payload := in.GetPayload()
	if len(payload) > 0 {
		if err := proto.Unmarshal(payload, req); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal request: %w", err)
		}
	}
	return req, nil
}

// UnmarshalResponse unmarshals the response proto message from the message.
// It uses the method name in the message to look up the Output type from the proto registry.
//
// This function should only be used by internal channel operations.
func UnmarshalResponse(out *Message) (proto.Message, error) {
	// get method descriptor from registry
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(out.GetMethod()))
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find method descriptor for %s", out.GetMethod())
	}
	methodDesc := desc.(protoreflect.MethodDescriptor)

	// get the response message type (Output type)
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(methodDesc.Output().FullName())
	if err != nil {
		return nil, fmt.Errorf("gorums: could not find message type %s", methodDesc.Output().FullName())
	}
	resp := msgType.New().Interface()

	// unmarshal message from the Message.Payload field
	payload := out.GetPayload()
	if len(payload) > 0 {
		if err := proto.Unmarshal(payload, resp); err != nil {
			return nil, fmt.Errorf("gorums: could not unmarshal response: %w", err)
		}
	}
	return resp, nil
}
