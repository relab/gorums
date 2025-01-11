package dtos

import (
	"context"
	"google.golang.org/protobuf/reflect/protoreflect"
	"time"
)

type Msg interface {
	GetBroadcastID() uint64
	GetMethod() string
	String() string
}

type BroadcastMsg struct {
	Ctx        context.Context
	Options    BroadcastOptions
	OriginAddr string
	Info       Info
}

func (msg *BroadcastMsg) GetBroadcastID() uint64 {
	return msg.Info.BroadcastID
}

func (msg *BroadcastMsg) GetMethod() string {
	return msg.Info.Method
}

func (msg *BroadcastMsg) String() string {
	return "broadcast"
}

type ReplyMsg struct {
	Info       Info
	ClientAddr string
	Err        error
}

func (r *ReplyMsg) GetBroadcastID() uint64 {
	return r.Info.BroadcastID
}

func (r *ReplyMsg) GetMethod() string {
	return "reply"
}

func (r *ReplyMsg) String() string {
	return "reply"
}

type Info struct {
	Message         protoreflect.ProtoMessage
	BroadcastID     uint64
	Method          string
	Addr            string
	OriginMethod    string
	OriginDigest    []byte
	OriginSignature []byte
	OriginPubKey    string
}

type Client struct {
	Addr    string
	SendMsg func(timeout time.Duration, dto *ReplyMsg) error
	Close   func() error
}

type BroadcastOptions struct {
	ServerAddresses  []string
	AllowDuplication bool
	SkipSelf         bool
	ProgressTo       string
	RelatedToReq     uint64
}
