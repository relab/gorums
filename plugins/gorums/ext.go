package gorums

import (
	gorumsproto "github.com/relab/gorums"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
)

func hasQRPCExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Qrpc)
	if err != nil {
		return false
	}
	if value == nil {
		return false
	}
	if value.(*bool) == nil {
		return false
	}
	return true
}

func hasMcastExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Multicast)
	if err != nil {
		return false
	}
	if value == nil {
		return false
	}
	if value.(*bool) == nil {
		return false
	}
	return true
}

func hasFutureExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_Future)
	if err != nil {
		return false
	}
	if value == nil {
		return false
	}
	if value.(*bool) == nil {
		return false
	}
	return true
}
