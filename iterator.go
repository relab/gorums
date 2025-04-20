package gorums

import (
	"errors"
	"iter"

	"google.golang.org/protobuf/proto"
)

// Response contains the values returned from a quorum call
// Msg is the proto message returned by the server
// Nid is the node id of the server
// Err may contain an error
type Response[messageType proto.Message] struct {
	Msg messageType
	Err error
	Nid uint32
}

// NewResponse initialized a response struct without needing to specify the message type
func NewResponse[messageType proto.Message](msg messageType, err error, nid uint32) Response[messageType] {
	return Response[messageType]{
		Msg: msg,
		Err: err,
		Nid: nid,
	}
}

// The Unpack methods lets you destructure the response in a single line
func (r Response[messageType]) Unpack() (messageType, error, uint32) {
	return r.Msg, r.Err, r.Nid
}

type Iterator[messageType proto.Message] iter.Seq[Response[messageType]]

// this method uses a function to keep/remove messages from a quorum call
// this can be used to verify the responses from servers before any further computation
func (iterator Iterator[messageType]) Filter(forward func(Response[messageType]) bool) Iterator[messageType] {
	return func(yield func(Response[messageType]) bool) {
		for resp := range iterator {
			if forward(resp) {
				if !yield(resp) {
					return
				}
			}
		}
	}
}

// this method uses a function to keep/remove messages from a quorum call
// this can be used to verify the responses from servers before any further computation
func (iterator Iterator[messageType]) IgnoreErrors() Iterator[messageType] {
	return func(yield func(Response[messageType]) bool) {
		for resp := range iterator {
			if resp.Err == nil {
				if !yield(resp) {
					return
				}
			}
		}
	}
}

// If you expect all servers to send the same reply,
// you can use GetQuorum to find the first message to occur
// a specified amount of times
func (iterator Iterator[messageType]) GetQuorum(quorum int) (messageType, error) {
	type replyFreq struct {
		val  messageType
		freq int
	}

	replyFreqs := make([]replyFreq, 0)

	for resp := range iterator {
		for i, replyFreq := range replyFreqs {
			if proto.Equal(replyFreq.val, resp.Msg) {
				replyFreqs[i].freq++
				if replyFreqs[i].freq >= quorum {
					return resp.Msg, nil
				}
			}
		}

		if quorum <= 1 {
			return resp.Msg, nil
		}
		replyFreqs = append(replyFreqs, replyFreq{resp.Msg, 1})
	}

	var noReply messageType
	return noReply, errors.New("quorum not found")
}
