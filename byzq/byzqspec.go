package byzq

import "fmt"

// defaultVal is returned by the register when no quorum is reached.
// It is considered safe to return this value, rather than returning
// a reply that does not constitute a quorum.
var defaultVal = Value{
	C: &Content{Timestamp: -1},
}

// ByzQ todo(doc) does something useful?
type ByzQ struct {
	n int // size of system
	f int // tolerable number of failures
	q int // quorum size
}

// NewByzQ returns a Byzantine masking quorum specification or nil and an error
// if the quorum requirements are not satisfied.
func NewByzQ(n int) (*ByzQ, error) {
	f := (n - 1) / 4
	if f < 1 {
		return nil, fmt.Errorf("Byzantine masking quorums require n>4f replicas; only got n=%d, yielding f=%d", n, f)
	}
	return &ByzQ{n, f, (n + 2*f) / 2}, nil
}

// ReadQF returns nil and false until the supplied replies
// constitute a Byzantine masking quorum, at which point the
// method returns a single Value and true.
func (bq *ByzQ) ReadQF(replies []*Value) (*Value, bool) {
	if len(replies) <= bq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}
	// filter out highest val that appears at least f times
	same := make(map[Content]int)
	highest := defaultVal
	for _, reply := range replies {
		same[*reply.C]++
		// select reply with highest timestamp if it has more than f replies
		if same[*reply.C] > bq.f && reply.C.Timestamp > highest.C.Timestamp {
			highest = *reply
		}
	}
	// returns the reply with the highest timestamp, or if no quorum for
	// the same timestamp-value pair has been found, the defaultVal is returned.
	return &highest, true
}

// WriteQF returns nil and false until the supplied replies
// constitute a Byzantine masking quorum, at which point the
// method returns a single write response and true.
func (bq *ByzQ) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	if len(replies) <= bq.q {
		return nil, false
	}
	return replies[0], true
}
