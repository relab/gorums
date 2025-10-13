package gorums

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
)

// DEADLOCK BUG REPRODUCTION TEST
//
// This test reliably reproduces a deadlock bug in channel.go where sender and
// receiver goroutines deadlock when the stream breaks during active communication.
//
// Location: channel.go
//
// Root Cause:
// 1. receiver() goroutine (line ~249) acquires streamMut.RLock() before calling RecvMsg()
// 2. When the stream breaks, RecvMsg() blocks waiting for data that will never arrive
// 3. Meanwhile, sender() goroutine detects the broken stream and calls reconnect(1) (line ~295)
// 4. reconnect() tries to acquire streamMut.Lock() (line ~308) but blocks because receiver holds RLock()
// 5. DEADLOCK: receiver won't release RLock until RecvMsg returns, but RecvMsg won't return
//    because the stream is broken. sender can't reconnect because it can't get the Lock.
//
// The issue: receiver() holds a read lock while doing a blocking I/O operation (RecvMsg)
// that can hang indefinitely when the stream is broken.
//
// Proposed Fixes (see DEADLOCK_BUG_ANALYSIS.md for details):
// Option 1 (Recommended): Don't hold the lock during RecvMsg()
//   - Release streamMut.RLock() before calling RecvMsg()
//   - Re-acquire it after RecvMsg() returns
//   - Check if stream was replaced while unlocked
//
// Option 2: Use a timeout/context in RecvMsg()
//   - Reduces the window but doesn't fully solve it
//
// Option 3: Redesign the locking strategy
//   - Use atomic operations or channels instead of mutexes
//   - Separate stream lifecycle management from I/O operations

// TestChannelDeadlock reliably reproduces the deadlock bug in channel.go.
//
// Scenario:
// 1. Start a server and establish a connection
// 2. Send a message to activate the stream
// 3. Stop the server to break the stream
// 4. Send multiple messages concurrently while stream is broken
//
// Expected Result:
// - Test FAILS with "DEADLOCK: Only received X/10 responses"
// - Only some goroutines complete before the deadlock occurs
// - Remaining goroutines are stuck waiting for locks
//
// The deadlock manifests when:
// - sender() goroutine is stuck in reconnect() waiting for streamMut.Lock()
// - receiver() goroutine is blocked in RecvMsg() while holding streamMut.RLock()
// - Client goroutines are blocked trying to enqueue messages
//
// Once the bug is fixed in channel.go, this test should pass with all 10 responses received.
func TestChannelDeadlock(t *testing.T) {
	srvAddr := "127.0.0.1:5011"
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())
	startServer()

	mgr := dummyMgr()
	defer mgr.Close()

	node, err := NewRawNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}

	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	// Ensure connection is up
	time.Sleep(50 * time.Millisecond)

	// Send a message to activate the stream
	sendRequest(t, node, 1, getCallOptions(E_Multicast, nil), 2*time.Second)

	// Break the stream by stopping server
	stopServer()
	time.Sleep(20 * time.Millisecond)

	// Send multiple messages concurrently when stream is broken
	// This triggers the deadlock between sender and receiver goroutines
	doneChan := make(chan bool, 10)

	for id := range 10 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			msgID := uint64(100 + id)
			md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
			req := request{
				ctx:  ctx,
				msg:  &Message{Metadata: md, Message: &mock.Request{}},
				opts: callOptions{},
			}

			// Try to enqueue with timeout
			select {
			case node.channel.sendQ <- req:
				// Successfully enqueued
				doneChan <- true
			case <-ctx.Done():
				// Timed out trying to enqueue (deadlock!)
				doneChan <- false
			}
		}()
	}

	// Wait for all goroutines to complete
	timeout := time.After(5 * time.Second)
	completed := 0
	successful := 0

	for completed < 10 {
		select {
		case success := <-doneChan:
			completed++
			if success {
				successful++
			}
		case <-timeout:
			t.Fatalf("DEADLOCK: Only %d/10 goroutines completed (%d successful). "+
				"Remaining goroutines are stuck trying to enqueue.",
				completed, successful)
		}
	}

	// If we reach here, all goroutines completed
	if successful == 10 {
		t.Log("SUCCESS: All 10 goroutines successfully enqueued - deadlock bug is fixed!")
	} else {
		t.Fatalf("PARTIAL DEADLOCK: %d/10 goroutines timed out trying to enqueue", 10-successful)
	}
}
