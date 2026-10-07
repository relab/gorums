package stream

import (
	"context"
	"runtime"
	"strconv"
	"strings"
)

// requestHandlerFunc adapts a function to [RequestHandler].
type requestHandlerFunc func(context.Context, *Message, func(), func(*Message))

// mockBidiStream is a bidirectional stream for testing inbound channels.
// Send echoes messages back via Recv (echo behavior).
// Call close() to simulate the stream being torn down.
type mockBidiStream struct {
	msgQ   chan *Message
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockBidiStream() *mockBidiStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockBidiStream{
		msgQ:   make(chan *Message, 16),
		ctx:    ctx,
		cancel: cancel,
	}
}

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
func drain(ch <-chan uint64) []uint64 {
	var got []uint64
	for {
		select {
		case v := <-ch:
			got = append(got, v)
		default:
			return got
		}
	}
}
