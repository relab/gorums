package broadcast

import (
	"context"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type BroadcastOptions struct {
	ServerAddresses      []string
	GossipPercentage     float32
	TTL                  int
	Deadline             time.Time
	OmitUniquenessChecks bool
	SkipSelf             bool
	ProgressTo           string
	RelatedToReq         uint64
}

const (
	BroadcastID string = "broadcastID"
	// special origin addr used in creating a broadcast request from a server
	ServerOriginAddr string = "server"
)

type ServerHandler func(ctx context.Context, in protoreflect.ProtoMessage, broadcastID uint64, originAddr, originMethod string, options BroadcastOptions, id uint32, addr string)

//type ClientHandler func(broadcastID uint64, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error)
