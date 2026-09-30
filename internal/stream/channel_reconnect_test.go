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

// TestChannelFailedStreamReleasesContext verifies that a failed NodeStream
// attempt cancels the context it created, so repeated attempts against an
// unreachable peer do not accumulate live child contexts of the connection.
func TestChannelFailedStreamReleasesContext(t *testing.T) {
	tc := setupChannelWithoutServer(t)
	for i := range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := tc.ensureConnectedNodeStream()
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("attempt %d: stream created against an unreachable peer", i)
			}
		case <-ctx.Done():
			t.Fatalf("attempt %d: ensureConnectedNodeStream did not return", i)
		}
		tc.streamMut.Lock()
		streamCtx := tc.streamCtx
		tc.streamMut.Unlock()
		if streamCtx.Err() == nil {
			t.Errorf("attempt %d: context of the failed stream is still live", i)
		}
	}
}
