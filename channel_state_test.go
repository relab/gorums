package gorums

import (
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestEnsureStreamCreatesStream verifies that ensureStream creates a stream when none exists.
func TestEnsureStreamCreatesStream(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	node.mgr = mgr
	if err := node.dial(); err != nil {
		t.Fatal(err)
	}

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
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	node.mgr = mgr
	if err := node.dial(); err != nil {
		t.Fatal(err)
	}

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
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	// After AddNode with server running, connection should be established
	// Wait a bit for connection to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.channel.isConnected() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !node.channel.isConnected() {
		t.Error("isConnected should return true when connection is Ready and stream exists")
	}

	// Clear the stream
	node.channel.streamMut.Lock()
	node.channel.gorumsStream = nil
	node.channel.streamMut.Unlock()

	// Now isConnected should return false (no stream)
	if node.channel.isConnected() {
		t.Error("isConnected should return false when stream is nil, even if connection is Ready")
	}
}
