package stream

import (
	"context"
	"testing"
	"time"
)

func TestChannelPauseReconnectSkipsLiveStream(t *testing.T) {
	connCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	live := newMockBidiStream()
	t.Cleanup(live.close)
	c := &Channel{
		stream:      live,
		streamReady: make(chan struct{}, 1),
		connCtx:     connCtx,
	}
	// A readiness signal is already queued, as when the sender created this
	// stream before the receiver reached the backoff wait.
	c.streamReady <- struct{}{}

	delay := 200 * time.Millisecond
	start := time.Now()
	if !c.pauseReconnect(&delay) {
		t.Fatal("pauseReconnect returned false")
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("pauseReconnect waited %v while a stream was already up", elapsed)
	}
	if delay != 200*time.Millisecond {
		t.Fatalf("delay = %v, want 200ms", delay)
	}
}
