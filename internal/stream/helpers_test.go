package stream

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// requestHandlerFunc adapts a function to [RequestHandler].
type requestHandlerFunc func(context.Context, *Message, func(), func(*Message))

// goroutineID returns the calling goroutine's ID, parsed from the header line
// of its stack trace ("goroutine N [...]").
func goroutineID() uint64 {
	var buf [64]byte
	fields := strings.Fields(string(buf[:runtime.Stack(buf[:], false)]))
	id, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		panic("goroutineID: " + err.Error())
	}
	return id
}

// drain returns the values buffered in ch without blocking.
func drain[T any](ch <-chan T) []T {
	var got []T
	for {
		select {
		case v := <-ch:
			got = append(got, v)
		default:
			return got
		}
	}
}

// messageIDs returns the message IDs of msgs in order.
func messageIDs(msgs []*Message) []uint64 {
	ids := make([]uint64, len(msgs))
	for i, msg := range msgs {
		ids[i] = msg.GetMessageSeqNo()
	}
	return ids
}

// lockedBuffer is a [strings.Builder] that is safe for concurrent use, so a
// test can read what a logger on another goroutine wrote.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

// Write implements [io.Writer].
func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the bytes written so far.
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
