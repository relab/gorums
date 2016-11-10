package gorums

import (
	gorumsproto "github.com/relab/gorums"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
)

func hasQuorumCallExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Qc)
	if err != nil {
		return false
	}
	return checkExtensionBoolValue(value)
}

func hasCorrectableExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Correctable)
	if err != nil {
		return false
	}
	return checkExtensionBoolValue(value)
}

func hasMulticastExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Multicast)
	if err != nil {
		return false
	}
	return checkExtensionBoolValue(value)
}

func hasFutureExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_QcFuture)
	if err != nil {
		return false
	}
	return checkExtensionBoolValue(value)
}

func checkExtensionBoolValue(value interface{}) bool {
	if value == nil {
		return false
	}
	if value.(*bool) == nil {
		return false
	}
	return true
}
