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
// the function can be private
func NewResponse[messageType proto.Message](msg messageType, err error, nid uint32) Response[messageType] {
	return Response[messageType]{
		Msg: msg,
		Err: err,
		Nid: nid,
	}
}

// Unpack destructures the response
func (r Response[messageType]) Unpack() (messageType, error, uint32) {
	return r.Msg, r.Err, r.Nid
}

type Iterator[messageType proto.Message] iter.Seq[Response[messageType]]

// Filter uses a function to keep/remove messages from a quorum call
// can be used to verify/filter responses from servers before computing quorum
func (iterator Iterator[messageType]) Filter(keep func(Response[messageType]) bool) Iterator[messageType] {
	return func(yield func(Response[messageType]) bool) {
		for resp := range iterator {
			if keep(resp) {
				if !yield(resp) {
					return
				}
			}
		}
	}
}

// IgnoreErrors removes the errors from the iterator
func (iterator Iterator[messageType]) IgnoreErrors() Iterator[messageType] {
	return func(yield func(Response[messageType]) bool) {
		for resp := range iterator {
			if resp.Err != nil {
				continue
			}
			if !yield(resp) {
				return
			}
		}
	}
}

// GetQuorum is an example function for an iterator method
// it returns a message if it occurs more than a specified amount of times
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
