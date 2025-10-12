package gorums

import (
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// TestEnsureConnectedIdleState verifies that ensureConnected() triggers
// connection when the ClientConn is in Idle state.
func TestEnsureConnectedIdleState(t *testing.T) {
	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	// Create a node without connecting
	node, err := NewRawNode("127.0.0.1:9999") // non-existent server
	if err != nil {
		t.Fatal(err)
	}

	node.mgr = mgr
	if err := node.dial(); err != nil {
		t.Fatal(err)
	}
	defer node.conn.Close()

	// Create channel but don't connect
	node.channel = newChannel(node)

	// Connection should be in Idle state initially
	initialState := node.conn.GetState()
	if initialState != connectivity.Idle {
		t.Logf("Note: Initial state is %v (expected Idle, but this may vary)", initialState)
	}

	// Call ensureConnected - should trigger connection attempt
	if err := node.channel.ensureConnected(); err != nil {
		t.Fatalf("ensureConnected failed: %v", err)
	}

	// State should have changed from Idle (connection attempt triggered)
	// Wait a bit for state transition
	time.Sleep(50 * time.Millisecond)
	newState := node.conn.GetState()

	// State should be Connecting, TransientFailure, or Ready (not Idle anymore)
	if newState == connectivity.Idle {
		t.Error("Connection state is still Idle after ensureConnected()")
	}
}

// TestEnsureConnectedShutdownState verifies that ensureConnected() returns
// error when the connection is shutdown.
func TestEnsureConnectedShutdownState(t *testing.T) {
	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	node, err := NewRawNode("127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}

	node.mgr = mgr
	if err := node.dial(); err != nil {
		t.Fatal(err)
	}

	// Close the connection to put it in Shutdown state
	node.conn.Close()

	// Wait for shutdown to complete
	time.Sleep(50 * time.Millisecond)

	node.channel = newChannel(node)

	// ensureConnected should return error for Shutdown state
	err = node.channel.ensureConnected()
	if err == nil {
		t.Error("ensureConnected should return error for Shutdown connection")
	}
}

// TestEnsureConnectedTransientFailure verifies that ensureConnected()
// attempts to reset backoff when connection is in TransientFailure.
func TestEnsureConnectedTransientFailure(t *testing.T) {
	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	defer mgr.Close()

	// Use non-existent server to trigger failure
	node, err := NewRawNode("127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}

	node.mgr = mgr
	if err := node.dial(); err != nil {
		t.Fatal(err)
	}
	defer node.conn.Close()

	node.channel = newChannel(node)

	// Trigger connection attempt
	node.conn.Connect()

	// Wait for TransientFailure state
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.conn.GetState() == connectivity.TransientFailure {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	state := node.conn.GetState()
	if state != connectivity.TransientFailure {
		t.Skipf("Could not reach TransientFailure state (got %v), skipping test", state)
	}

	// ensureConnected should handle TransientFailure by calling ResetConnectBackoff
	if err := node.channel.ensureConnected(); err != nil {
		t.Fatalf("ensureConnected failed on TransientFailure: %v", err)
	}

	// Verify the function returned successfully (doesn't verify retry happens,
	// but ensures no panic or error)
	t.Log("ensureConnected handled TransientFailure state successfully")
}

// TestEnsureStreamCreatesStream verifies that ensureStream creates a new
// stream if one doesn't exist.
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
