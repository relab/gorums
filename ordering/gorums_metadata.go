package ordering

import (
	"context"
	"errors"

	"google.golang.org/grpc/metadata"
)

// NewGorumsMetadata creates a new Gorums metadata object for the given method
// and client message ID. It also appends any client-specific metadata from the
// context to the Gorums metadata object.
//
// This is used to pass client-specific metadata to the server via Gorums.
// This method should be used by generated code only.
func NewGorumsMetadata(ctx context.Context, msgID uint64, method string) *Metadata {
	gorumsMetadata := Metadata_builder{MessageID: msgID, Method: method}
	md, _ := metadata.FromOutgoingContext(ctx)
	for k, vv := range md {
		for _, v := range vv {
			entry := MetadataEntry_builder{Key: k, Value: v}.Build()
			gorumsMetadata.Entry = append(gorumsMetadata.Entry, entry)
		}
	}
	return gorumsMetadata.Build()
}

// AppendToIncomingContext appends client-specific metadata from the
// Gorums metadata object to the incoming context.
//
// This is used to pass client-specific metadata from the Gorums runtime
// to the server implementation.
// This method should be used by generated code only.
func (x *Metadata) AppendToIncomingContext(ctx context.Context) context.Context {
	existingMD, _ := metadata.FromIncomingContext(ctx)
	newMD := existingMD.Copy() // copy to avoid mutating the original
	for _, entry := range x.GetEntry() {
		newMD.Append(entry.GetKey(), entry.GetValue())
	}
	return metadata.NewIncomingContext(ctx, newMD)
}

func (x *Metadata) GetValidAuthMsg() (*AuthMsg, error) {
	if x == nil {
		return nil, errors.New("metadata cannot be nil")
	}
	authMsg := x.GetAuthMsg()
	if authMsg == nil {
		return nil, errors.New("missing AuthMsg")
	}
	if authMsg.GetSignature() == nil {
		return nil, errors.New("missing signature")
	}
	if authMsg.GetPublicKey() == "" {
		return nil, errors.New("missing publicKey")
	}
	return authMsg, nil
}
