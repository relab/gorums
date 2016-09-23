package byzq

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"
)

// AuthDataQ is the quorum specification for the Authenticated-Data Byzantine quorum
// algorithm described in RSDP, Algorithm 4.15, page 181.
type AuthDataQ struct {
	n    int               // size of system
	f    int               // tolerable number of failures
	q    int               // quorum size
	priv *ecdsa.PrivateKey // writer's private key for signing
	pub  *ecdsa.PublicKey  // public key of the writer (used by readers)
	wts  int64             // writer's timestamp
}

// NewAuthDataQ returns a Byzantine masking quorum specification or nil and an error
// if the quorum requirements are not satisfied.
func NewAuthDataQ(n int, priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) (*AuthDataQ, error) {
	f := (n - 1) / 3
	if f < 1 {
		return nil, fmt.Errorf("Byzantine quorum require n>3f replicas; only got n=%d, yielding f=%d", n, f)
	}
	return &AuthDataQ{n, f, (n + f) / 2, priv, pub, 0}, nil
}

// IncWTS updates the writer's timestamp wts. This is not thread safe.
func (aq *AuthDataQ) IncWTS() int64 {
	aq.wts++
	return aq.wts
}

// NewTS reads the system clock and updates the writer's timestamp wts. This is not thread safe.
func (aq *AuthDataQ) NewTS() int64 {
	aq.wts = time.Now().UnixNano()
	return aq.wts
}

// Sign signs the provided content and returns a value to be passed into Write.
// (This function must currently be exported since our writer client code is not
// in the byzq package.)
func (aq *AuthDataQ) Sign(content *Content) (*Value, error) {
	msg, err := content.Marshal()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(msg)
	r, s, err := ecdsa.Sign(rand.Reader, aq.priv, hash[:])
	if err != nil {
		return nil, err
	}
	return &Value{C: content, SignatureR: r.Bytes(), SignatureS: s.Bytes()}, nil
}

func (aq *AuthDataQ) verify(reply *Value) bool {
	msg, err := reply.C.Marshal()
	if err != nil {
		log.Printf("failed to marshal msg for verify: %v", err)
		return false
	}
	msgHash := sha256.Sum256(msg)
	r := new(big.Int).SetBytes(reply.SignatureR)
	s := new(big.Int).SetBytes(reply.SignatureS)
	return ecdsa.Verify(aq.pub, msgHash[:], r, s)
}

// ReadQF returns nil and false until the supplied replies
// constitute a Byzantine quorum, at which point the method returns the
// single highest value and true.
func (aq *AuthDataQ) ReadQF(replies []*Value) (*Value, bool) {
	if len(replies) <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}
	var highest *Value
	for _, reply := range replies {
		if highest != nil && reply.C.Timestamp <= highest.C.Timestamp {
			continue
		}
		highest = reply
	}
	// returns reply with the highest timestamp, or nil if no replies were verified
	return highest, true
}

// SequentialVerifyReadQF returns nil and false until the supplied replies
// constitute a Byzantine quorum, at which point the method returns the
// single highest value and true.
func (aq *AuthDataQ) SequentialVerifyReadQF(replies []*Value) (*Value, bool) {
	if len(replies) <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}
	var highest *Value
	for _, reply := range replies {
		if aq.verify(reply) {
			if highest != nil && reply.C.Timestamp <= highest.C.Timestamp {
				continue
			}
			highest = reply
		}
	}
	// returns reply with the highest timestamp, or nil if no replies were verified
	return highest, true
}

// ConcurrentVerifyWGReadQF returns nil and false until the supplied replies
// constitute a Byzantine quorum, at which point the method returns the
// single highest value and true.
func (aq *AuthDataQ) ConcurrentVerifyWGReadQF(replies []*Value) (*Value, bool) {
	if len(replies) <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}
	verified := make([]bool, len(replies))
	wg := &sync.WaitGroup{}
	for i, reply := range replies {
		wg.Add(1)
		go func(i int, r *Value) {
			verified[i] = aq.verify(r)
			wg.Done()
		}(i, reply)
	}
	wg.Wait()
	cnt := 0
	var highest *Value
	for i, v := range verified {
		if !v {
			// some signature could not be verified:
			cnt++
			if len(replies)-cnt <= aq.q {
				return nil, false
			}
		}
		if highest != nil && replies[i].C.Timestamp <= highest.C.Timestamp {
			continue
		}
		highest = replies[i]
	}

	// returns reply with the highest timestamp, or nil if no replies were verified
	return highest, true
}

// ConcurrentVerifyIndexChanReadQF returns nil and false until the supplied replies
// constitute a Byzantine quorum, at which point the method returns the
// single highest value and true.
func (aq *AuthDataQ) ConcurrentVerifyIndexChanReadQF(replies []*Value) (*Value, bool) {
	if len(replies) <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}

	veriresult := make(chan int, len(replies))
	for i, reply := range replies {
		go func(i int, r *Value) {
			if !aq.verify(r) {
				i = -1
			}
			veriresult <- i
		}(i, reply)
	}

	cnt := 0
	var highest *Value
	for j := 0; j < len(replies); j++ {
		i := <-veriresult
		if i == -1 {
			// some signature could not be verified:
			cnt++
			if len(replies)-cnt <= aq.q {
				return nil, false
			}
		}
		if highest != nil && replies[i].C.Timestamp <= highest.C.Timestamp {
			continue
		}
		highest = replies[i]
	}
	// returns reply with the highest timestamp, or nil if no replies were verified
	return highest, true
}

// VerfiyLastReplyFirstReadQF returns nil and false until the supplied replies
// constitute a Byzantine quorum, at which point the method returns the
// single highest value and true.
func (aq *AuthDataQ) VerfiyLastReplyFirstReadQF(replies []*Value) (*Value, bool) {
	if len(replies) < 1 {
		return nil, false
	}
	if !aq.verify(replies[len(replies)-1]) {
		// return if last reply failed to verify
		replies[len(replies)-1] = nil
		return nil, false
	}
	if len(replies) <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}

	var highest *Value
	cntnotnil := 0
	for _, reply := range replies {
		if reply == nil {
			continue
		}
		cntnotnil++
		// select reply with highest timestamp
		if highest != nil && reply.C.Timestamp <= highest.C.Timestamp {
			continue
		}
		highest = reply
	}

	if cntnotnil <= aq.q {
		// not enough replies yet; need at least bq.q=(n+2f)/2 replies
		return nil, false
	}
	// returns reply with the highest timestamp, or nil if no replies were verified
	return highest, true
}

// WriteQF returns nil and false until it is possible to check for a quorum.
// If enough replies with the same timestamp is found, we return true.
func (aq *AuthDataQ) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	if len(replies) <= aq.q {
		return nil, false
	}
	cnt := 0
	var reply *WriteResponse
	for _, r := range replies {
		if aq.wts == r.Timestamp {
			cnt++
			reply = r
		}
	}
	if cnt < aq.q {
		return nil, false
	}
	return reply, true
}
