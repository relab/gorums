package gorums

import (
	gorumsproto "github.com/relab/gorums"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
)

func qcName() string {
	return gorumsproto.E_Qc.Name
}

func futureName() string {
	return gorumsproto.E_QcFuture.Name
}

func corrName() string {
	return gorumsproto.E_Correctable.Name
}

func corrPrName() string {
	return gorumsproto.E_CorrectablePr.Name
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

func hasCorrectablePRExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_CorrectablePr)
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

func hasQFWithReqExtension(method *descriptor.MethodDescriptorProto) bool {
	if method.Options == nil {
		return false
	}
	value, err := proto.GetExtension(method.Options, gorumsproto.E_QfWithReq)
	if err != nil {
		return false
	}
	return checkExtensionBoolValue(value)
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

// TODO(meling): Eliminate the follwing func by using the GetBoolExtension() as
// shown in the hasPerNodeArgExtension() func.
func checkExtensionBoolValue(value interface{}) bool {
	if value == nil {
		return false
	}
	if value.(*bool) == nil {
		return false
	}
	return true
}
