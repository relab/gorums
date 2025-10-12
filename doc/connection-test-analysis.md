# Connection Management Test Analysis

## Overview

This document analyzes the tests related to connection management in Gorums to determine which tests depend on synchronous connection establishment in `AddNode()` and how they can be adapted for non-blocking behavior.

## Current Behavior

Currently, `AddNode()` calls `node.connect(m)` which:

1. Creates a gRPC ClientConn via `node.dial()` (non-blocking - uses `grpc.NewClient`)
2. Creates a channel and starts sender goroutine
3. Calls `channel.connect()` which attempts to:
   - Call `ensureConnected()` (non-blocking - just triggers connection)
   - Call `ensureStream()` (blocking - tries to create stream immediately)

## Tests That Check Connection After AddNode

### 1. TestChannelSuccessfulConnection (channel_test.go:67)

**Current behavior:**

```go
if err = mgr.AddNode(node); err != nil {
    t.Fatal(err)
}
if !node.channel.isConnected() {
    t.Fatal("a connection could not be made to a live node")
}
```

**Issue:** Immediately checks `isConnected()` after `AddNode()`, expecting synchronous connection.

**Proposed fix:**

- Option A: Trigger connection by sending a dummy message
- Option B: Wait in a retry loop for connection (with timeout)
- Option C: Remove this check and test connection via actual RPC call

**Recommendation:** Option B - Add a helper that waits for connection with timeout, demonstrating proper usage pattern for users who want to verify connectivity before proceeding.

### 2. TestChannelUnsuccessfulConnection (channel_test.go:92)

**Current behavior:**

```go
// no servers are listening on the given address
node, err := NewRawNode("127.0.0.1:5000")
if err = mgr.AddNode(node); err != nil {
    t.Fatal(err)
}
if node.conn == nil {
    t.Fatal("connection should not be nil")
}
```

**Issue:** None! This test only checks that `conn != nil`, which will be true even with non-blocking AddNode (since `dial()` creates the ClientConn).

**Proposed fix:** No change needed.

### 3. TestServerCallback (server_test.go:30)

**Current behavior:**

```go
if err = mgr.AddNode(node); err != nil {
    t.Fatal(err)
}

select {
case <-time.After(100 * time.Millisecond):
case <-signal:
}

if message != "hello" {
    t.Errorf("incorrect message: got '%s', want 'hello'", message)
}
```

**Issue:** Expects server callback to be triggered after AddNode. The test waits 100ms which might not be enough if connection is truly lazy.

**Proposed fix:** The server callback happens when stream is established. If `AddNode()` doesn't establish stream, we need to trigger it. However, looking at the test, it seems the callback might be triggered by the client sending metadata on stream creation. Need to verify if this is a server-initiated callback or client-triggered.

**Recommendation:** Send a dummy message after AddNode to trigger stream establishment, or increase timeout and document that connection is lazy.

### 4. TestIsConnectedRequiresBothReadyAndStream (channel_state_test.go:247)

**Current behavior:**

```go
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
```

**Issue:** This test already has a retry loop! It's already adapted for potential async behavior.

**Proposed fix:** No change needed - test is already robust.

## Tests That Trigger Connection Via Message Sending

### 1. TestChannelCreation (channel_test.go:38)

**Current behavior:**

```go
node.connect(mgr)
// ... creates a request and enqueues it
node.channel.enqueue(req, replyChan, false)
```

**Issue:** None - explicitly calls `connect()` and then sends message.

**Proposed fix:** No change needed.

### 2. TestChannelReconnection (channel_test.go:113)

**Current behavior:**

```go
node.connect(mgr)

// send first message when server is down
go func() {
    ctx := context.Background()
    md := ordering.NewGorumsMetadata(ctx, 1, handlerName)
    req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}}
    node.channel.enqueue(req, replyChan1, false)
}()
```

**Issue:** None - explicitly calls `connect()` first, then tests message sending.

**Proposed fix:** No change needed.

## Connection Establishment Triggers

Based on code analysis, connection is established when:

1. **`channel.connect()` is called** - Attempts immediate connection
2. **`sender()` processes first message** - Calls `ensureConnected()` + `ensureStream()`
3. **`channel.enqueue()` sends to sendQ** - Sender picks it up and establishes connection

## Recommendation for Non-Blocking AddNode

### Updated AddNode Behavior

```go
func (m *RawManager) AddNode(node *RawNode) error {
    // ... validation ...

    node.mgr = m
    if !m.opts.noConnect {
        // Create ClientConn (non-blocking - grpc.NewClient returns immediately)
        if err := node.dial(); err != nil {
            return err
        }
        // Start channel and sender goroutine (doesn't establish stream yet)
        node.channel = newChannel(node)
    }

    // Add to pool
    m.mu.Lock()
    m.lookup[node.id] = node
    m.nodes = append(m.nodes, node)
    m.mu.Unlock()

    return nil
}
```

### Helper Function for Tests

```go
// waitForConnection waits for node to establish connection, with timeout
func waitForConnection(t *testing.T, node *RawNode, timeout time.Duration) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if node.channel.isConnected() {
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatal("timeout waiting for connection")
}
```

### Test Changes Required

1. **TestChannelSuccessfulConnection** - Use helper or send dummy message:

```go
if err = mgr.AddNode(node); err != nil {
    t.Fatal(err)
}

// Trigger connection by sending a message or wait for it
// Option 1: Wait with helper
waitForConnection(t, node, 2*time.Second)

// Option 2: Send dummy message to trigger connection
ctx := context.Background()
md := ordering.NewGorumsMetadata(ctx, 1, handlerName)
req := request{ctx: ctx, msg: &Message{Metadata: md, Message: &mock.Request{}}}
replyChan := make(chan response, 1)
node.channel.enqueue(req, replyChan, false)
<-replyChan // Wait for response (triggers connection)

if !node.channel.isConnected() {
    t.Fatal("a connection could not be made to a live node")
}
```

2. **TestServerCallback** - Send explicit message or increase timeout:

```go
if err = mgr.AddNode(node); err != nil {
    t.Fatal(err)
}

// Send a dummy message to trigger connection and callback
// OR just increase timeout and document lazy connection
select {
case <-time.After(500 * time.Millisecond): // Increased timeout
case <-signal:
}
```

## Benefits of Non-Blocking AddNode

1. **No hanging on unreachable nodes** - AddNode returns immediately even if server is down
2. **Better for bulk operations** - Can add many nodes quickly without waiting for each
3. **Consistent with gRPC** - Matches grpc.NewClient's non-blocking behavior
4. **Automatic retry** - Sender goroutine handles connection on first message
5. **Simpler error handling** - No need to handle connection failures during setup

## Compatibility Considerations

- **Breaking change**: Code that assumes immediate connectivity after AddNode will break
- **Mitigation**: Most users send messages immediately anyway, which triggers connection
- **Documentation**: Clear docs that connection is lazy and established on first use
