package processor

import (
	"context"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type RequestDto struct {
	BroadcastID     uint64
	IsServer        bool
	OriginAddr      string
	OriginMethod    string
	OriginPubKey    string
	OriginSignature []byte
	OriginDigest    []byte
	ViewNumber      uint64
	SenderAddr      string
	CurrentMethod   string
	SendFn          func(resp protoreflect.ProtoMessage, err error) error
	Ctx             context.Context
	CancelCtx       context.CancelFunc
	Run             func(EnqueueMsg)
}
