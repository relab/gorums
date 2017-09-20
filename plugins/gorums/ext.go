package gorums

import (
	gorumsproto "github.com/relab/gorums"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
)

// func qcName() string {
// 	return gorumsproto.E_Qc.Name
// }

// func futureName() string {
// 	return gorumsproto.E_QcFuture.Name
// }

// func corrName() string {
// 	return gorumsproto.E_Correctable.Name
// }

func corrPrName() string {
	return gorumsproto.E_CorrectableStream.Name
}

func mcastName() string {
	return gorumsproto.E_Multicast.Name
}

func qfreqName() string {
	return gorumsproto.E_QfWithReq.Name
}

func custRetName() string {
	return gorumsproto.E_CustomReturnType.Name
}

func hasQuorumCallExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_Qc, false)
}

func hasCorrectableExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_Correctable, false)
}

func hasCorrectableStreamExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_CorrectableStream, false)
}

func hasMulticastExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_Multicast, false)
}

func hasFutureExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_QcFuture, false)
}

func hasQFWithReqExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_QfWithReq, false)
}

func hasPerNodeArgExtension(method *descriptor.MethodDescriptorProto) bool {
	return method != nil && proto.GetBoolExtension(method.Options, gorumsproto.E_PerNodeArg, false)
}

func getCustomReturnTypeExtension(method *descriptor.MethodDescriptorProto) string {
	if method == nil {
		return ""
	}
	if method.Options != nil {
		v, err := proto.GetExtension(method.Options, gorumsproto.E_CustomReturnType)
		if err == nil && v.(*string) != nil {
			return *(v.(*string))
		}
	}
	return ""
}
