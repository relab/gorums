package gorums

import (
	"testing"
)

// TestEnsureStreamCreatesStream verifies that ensureStream creates a stream when none exists.
func TestEnsureStreamCreatesStream(t *testing.T) {
	node := newNodeWithServer(t, 0)

	// Replace channel with fresh one that has no stream
	node.channel = newChannel(node)

	// Initially no stream
	node.channel.streamMut.Lock()
	initialStream := node.channel.gorumsStream
	node.channel.streamMut.Unlock()

	if initialStream != nil {
		t.Fatal("Stream should be nil initially")
	}

	// ensureStream should create stream
	if err := node.channel.ensureStream(); err != nil {
		t.Fatalf("ensureStream failed: %v", err)
	}

	// Verify stream was created
	node.channel.streamMut.Lock()
	finalStream := node.channel.gorumsStream
	node.channel.streamMut.Unlock()

	if finalStream == nil {
		t.Error("Stream was not created by ensureStream")
	}
}

// TestEnsureStreamIdempotent verifies that calling ensureStream multiple
// times doesn't create multiple streams.
func TestEnsureStreamIdempotent(t *testing.T) {
	node := newNodeWithServer(t, 0)

	// Replace channel with fresh one that has no stream
	node.channel = newChannel(node)

	// Create first stream
	if err := node.channel.ensureStream(); err != nil {
		t.Fatalf("First ensureStream failed: %v", err)
	}

	node.channel.streamMut.Lock()
	firstStream := node.channel.gorumsStream
	node.channel.streamMut.Unlock()

	// Call ensureStream again
	if err := node.channel.ensureStream(); err != nil {
		t.Fatalf("Second ensureStream failed: %v", err)
	}

	node.channel.streamMut.Lock()
	secondStream := node.channel.gorumsStream
	node.channel.streamMut.Unlock()

	// Should be the same stream
	if firstStream != secondStream {
		t.Error("ensureStream created a new stream instead of reusing existing one")
	}
}

// TestIsConnectedRequiresBothReadyAndStream verifies that isConnected()
// returns true only when both connection is Ready AND stream exists.
func TestIsConnectedRequiresBothReadyAndStream(t *testing.T) {
	node := newNodeWithServer(t, 0)
	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	node.channel.clearStream()

	if node.channel.isConnected() {
		t.Error("isConnected should return false after clearing the stream")
	}
}
