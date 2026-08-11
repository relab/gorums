package impl

import (
	"fmt"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// unmarshalResponse unmarshals the response proto message from the message.
// It uses the method name in the message to look up the Output type from the proto registry.
func unmarshalResponse(out *stream.Message) (proto.Message, error) {
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
