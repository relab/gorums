package dev

import "google.golang.org/protobuf/proto"

var (
	marshaler = proto.MarshalOptions{
		AllowPartial:  true,
		Deterministic: true,
	}
	unmarshaler = proto.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
)

func appendIfNotPresent(set []uint32, x uint32) []uint32 {
	for _, y := range set {
		if y == x {
			return set
		}
	}
	return append(set, x)
}
