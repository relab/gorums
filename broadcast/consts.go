package broadcast

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastOptions struct {
	ServerAddresses  []string
	AllowDuplication bool
	SkipSelf         bool
	ProgressTo       string
	RelatedToReq     uint64
}

const (
	BroadcastID string = "broadcastID"
	// special origin addr used in creating a broadcast request from a server
	ServerOriginAddr string = "server"
)

type ServerHandler func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string)
