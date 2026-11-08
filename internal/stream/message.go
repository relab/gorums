package stream

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// NewRequest creates a request [Message] for the given method, message ID, and
// proto message. The proto message is marshaled into the message payload, and
// any client-specific metadata in ctx is extracted and appended to the message.
// If the proto message is nil, the message payload will be empty.
func NewRequest(ctx context.Context, msgID uint64, method string, msg proto.Message) (*Message, error) {
	// Marshal the message to bytes (nil message returns nil bytes and no error)
	payload, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return NewRequestFromPayload(ctx, msgID, method, payload), nil
}

// NewRequestFromPayload creates a request [Message] from already-marshaled
// protobuf payload bytes.
func NewRequestFromPayload(ctx context.Context, msgID uint64, method string, payload []byte) *Message {
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
	return msgBuilder.Build()
}

// NewResponse returns the response [Message] to the request in. It carries the
// request's message ID and method, and either the status of err or the
// marshaled resp. A marshal failure becomes the response status.
func NewResponse(in *Message, resp proto.Message, err error) *Message {
	out := Message_builder{
		MessageSeqNo: in.GetMessageSeqNo(), // routes the response to the pending call
		Method:       in.GetMethod(),       // selects the response type on the client
	}.Build()
	if err == nil && resp != nil {
		var payload []byte
		if payload, err = proto.Marshal(resp); err == nil {
			out.SetPayload(payload)
		}
	}
	if err != nil {
		errStatus, ok := status.FromError(err)
		if !ok {
			errStatus = status.New(codes.Unknown, err.Error())
		}
		out.SetStatus(errStatus.Proto())
	}
	return out
}

// appendToIncomingContext appends client-specific metadata from the [Message] proto message
// to the incoming gRPC context, allowing server implementations to extract and use said
// metadata directly from the server method's context. It never modifies ctx's own metadata,
// and it returns ctx unchanged when the message has no metadata entries.
//
// This method is intended for Gorums internal use.
func (x *Message) appendToIncomingContext(ctx context.Context) context.Context {
	entries := x.GetEntry()
	if len(entries) == 0 {
		return ctx
	}
	// FromIncomingContext returns a copy, so appending leaves ctx's metadata unchanged.
	md, _ := metadata.FromIncomingContext(ctx)
	if md == nil {
		md = make(metadata.MD, len(entries))
	}
	for _, entry := range entries {
		md.Append(entry.GetKey(), entry.GetValue())
	}
	return metadata.NewIncomingContext(ctx, md)
}

// errorStatus returns the error encoded in the message status, or nil if the
// message carries no status.
func (x *Message) errorStatus() error {
	s := x.GetStatus()
	if s == nil {
		return nil
	}
	return status.ErrorProto(s)
}

// serverSequenceNumberBitMask is the high-bit partition used to distinguish
// server-initiated message IDs from client-initiated message IDs on a shared
// bidirectional stream.
const serverSequenceNumberBitMask = uint64(1) << 63

// ServerSequenceNumber marks seqNo as server-initiated.
func ServerSequenceNumber(seqNo uint64) uint64 {
	return seqNo | serverSequenceNumberBitMask
}

// isServerSequenceNumber reports whether seqNo belongs to the server-initiated
// message ID space.
func isServerSequenceNumber(seqNo uint64) bool {
	return seqNo&serverSequenceNumberBitMask != 0
}
