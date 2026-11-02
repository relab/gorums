package impl

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrIncomplete is exported and documented as gorums.ErrIncomplete.
var ErrIncomplete = errors.New("incomplete call")

// ErrSendFailure is exported and documented as gorums.ErrSendFailure.
var ErrSendFailure = errors.New("send failure")

// ErrSkipNode is exported and documented as gorums.ErrSkipNode.
var ErrSkipNode = errors.New("skip node")

// ErrTypeMismatch is exported and documented as gorums.ErrTypeMismatch.
var ErrTypeMismatch = errors.New("response type mismatch")

// ErrNoNode is exported and documented as gorums.ErrNoNode.
var ErrNoNode = status.Error(codes.InvalidArgument, "no such node")

// ErrEmptyConfig is exported and documented as gorums.ErrEmptyConfig.
var ErrEmptyConfig = status.Error(codes.InvalidArgument, "empty configuration")
