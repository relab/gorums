// Package dtos implements all data transfer objects used from outside the broadcast implementation context.
package dtos

import (
	"context"
	"google.golang.org/protobuf/reflect/protoreflect"
	"time"
)

// Msg defines the message sent from a server to another server or client. The messages should be sent by the router.
type Msg interface {
	GetBroadcastID() uint64
	GetMethod() string
	String() string
}

// BroadcastMsg is a data transfer object of a message received by another server or client.
type BroadcastMsg struct {
	Ctx     context.Context
	Options BroadcastOptions
	// The address of the client or server that originated the broadcast request
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

// ReplyMsg is similar to BroadcastMsg, but is strictly used for replying to a client.
type ReplyMsg struct {
	Info Info
	// The address of the client that originated the broadcast request
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

// Info contains data pertaining to the current message such as routing information, contents, and which server handler
// should receive the message.
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

// Client is a data structure used when sending a reply to a client.
type Client struct {
	Addr    string
	SendMsg func(timeout time.Duration, dto *ReplyMsg) error
	Close   func() error
}

// BroadcastOptions is used to configure a particular broadcast, e.g. by only broadcasting to a subset of the servers in
// a view.
type BroadcastOptions struct {
	ServerAddresses  []string
	AllowDuplication bool
	SkipSelf         bool
	ProgressTo       string
	RelatedToReq     uint64
}
