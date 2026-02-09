package stream

import (
	"context"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// NewMessage creates a new [Message] proto message for the given method and message ID.
// If a non-nil proto message is provided, it is marshaled and included in the message payload.
// This function also extracts any client-specific metadata from the context and appends
// it to the message, allowing client-specific metadata to be passed to the server.
//
// This method is intended for Gorums internal use.
// This function is used on the client-side to create outgoing request messages.
func NewMessage(ctx context.Context, msgID uint64, method string, msg proto.Message) (*Message, error) {
	// Marshal the message to bytes (nil message returns nil bytes and no error)
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	msgBuilder := Message_builder{
		MessageSeqNo: msgID,
		Method:       method,
		Payload:      payload,
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	for k, vv := range md {
		for _, v := range vv {
			entry := MetadataEntry_builder{Key: k, Value: v}.Build()
			msgBuilder.Entry = append(msgBuilder.Entry, entry)
		}
	}
	return msgBuilder.Build(), nil
}

// AppendToIncomingContext appends client-specific metadata from the [Message] proto message
// to the incoming gRPC context, allowing server implementations to extract and use said
// metadata directly from the server method's context.
//
// This method is intended for Gorums internal use.
func (x *Message) AppendToIncomingContext(ctx context.Context) context.Context {
	existingMD, _ := metadata.FromIncomingContext(ctx)
	newMD := existingMD.Copy() // copy to avoid mutating the original
	for _, entry := range x.GetEntry() {
		newMD.Append(entry.GetKey(), entry.GetValue())
	}
	return metadata.NewIncomingContext(ctx, newMD)
}
