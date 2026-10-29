# Migration Guide: Gorums v2 to v3

Status: retained as a version-specific historical migration guide.
It does not describe the later API redesign; that missing guide is tracked in [issue-redesign-migration-guide.md](issue-redesign-migration-guide.md).
Its call examples use the generated service clients.
To migrate code from the generated call functions to service clients, see [Function API to Service Clients](#function-api-to-service-clients).

This guide helps you migrate from Gorums v2 (QuorumSpec-based design) to v3 (iterator-based design with generic functions).

## Overview of Changes

Gorums v3 introduces a fundamental redesign focused on:

- **Type aliases** instead of generated concrete types
- **Generic functions** with terminal methods instead of QuorumSpec interface
- **Iterator-based response aggregation** for flexible custom logic
- **Service clients** that bind the generated call methods to a configuration or a node
- **Custom return types** support for aggregation functions

### Why These Changes?

1. **Simpler generated code**: Type aliases reduce code duplication
2. **More flexibility**: Iterators enable progressive response handling
3. **Better Go generics usage**: Leverages Go 1.18+ generics properly
4. **Cleaner API**: Terminal methods (`.Majority()`, `.First()`, `.All()`) for common patterns
5. **Works around Go limitations**: Go doesn't support generic methods ([Go FAQ](https://go.dev/doc/faq#generic_methods))

## Quick Comparison

### Legacy v2 Design

```go
// QuorumSpec interface generated per service
type QuorumSpec interface {
    ReadQF(req *ReadRequest, replies map[uint32]*State) (*State, bool)
    WriteQF(req *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool)
}

// User implements QuorumSpec
type QSpec struct{ quorumSize int }

func (qs *QSpec) ReadQF(_ *ReadRequest, replies map[uint32]*State) (*State, bool) {
    if len(replies) < qs.quorumSize {
        return nil, false
    }
    return newestState(replies), true
}

// Configuration created with QuorumSpec
cfg, _ := mgr.NewConfiguration(&QSpec{2}, gorums.WithNodeList(addrs))

// Method calls on Configuration
reply, err := cfg.Read(ctx, &ReadRequest{})
```

### New v3 Design

```go
// No QuorumSpec interface needed

// Standalone aggregation function
func newestValue(responses *gorums.Responses[*ReadResponse]) (*ReadResponse, error) {
    var newest *ReadResponse
    for resp := range responses.Results() {
        if resp.Err != nil {
            continue
        }
        if newest == nil || resp.Value.GetTime().After(newest.GetTime()) {
            newest = resp.Value
        }
    }
    if newest == nil {
        return nil, gorums.ErrIncomplete
    }
    return newest, nil
}

// Configuration created without QuorumSpec
config, _ := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))

// Service client bound to the configuration
client := NewStorageClient(config)
call := client.ReadQC(ctx, &ReadRequest{})

// Option 1: Use custom aggregation
reply, err := newestValue(call.Responses)

// Option 2: Use built-in terminal method
reply, err := client.ReadQC(ctx, &ReadRequest{}).Majority()
```

## Migration Steps

### Step 1: Regenerate Protocol Buffers

Regenerate all `.proto` files with the new `protoc-gen-gorums` plugin:

```bash
make genproto
```

This generates new `*_gorums.pb.go` files with:

- Type aliases for `Manager`, `Configuration`, `Node`
- Service client types with a method for each call (e.g., `client.ReadQC`, `client.WriteQC`)
- Terminal methods on `*gorums.Responses[T]`

### Step 2: Update Configuration Creation

Remove QuorumSpec from configuration creation.

**Before:**

```go
mgr := NewManager(gorums.WithDialOptions(...))
cfg, err := mgr.NewConfiguration(
    &QSpec{quorumSize: 2},  // ❌ Remove QuorumSpec
    gorums.WithNodeList(addrs),
)
```

**After:**

```go
mgr := gorums.NewManager(gorums.WithDialOptions(...))
cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
```

Or use the convenience function that creates both:

```go
cfg, err := gorums.NewConfig(
    gorums.WithNodeList(addrs),
    gorums.WithDialOptions(...),
)
```

### Step 3: Convert QuorumSpec Methods to Aggregation Functions

Transform your QuorumSpec methods into standalone aggregation functions that work with iterators.

**Before:**

```go
type QSpec struct{ quorumSize int }

func (qs *QSpec) ReadQF(_ *ReadRequest, replies map[uint32]*State) (*State, bool) {
    if len(replies) < qs.quorumSize {
        return nil, false
    }
    var newest *State
    for _, s := range replies {
        if newest == nil || s.GetTimestamp() > newest.GetTimestamp() {
            newest = s
        }
    }
    return newest, true
}
```

**After:**

```go
func newestState(responses *gorums.Responses[*State]) (*State, error) {
    var newest *State
    for resp := range responses.Results() {
        if resp.Err != nil {
            continue  // Skip failed responses
        }
        if newest == nil || resp.Value.GetTimestamp() > newest.GetTimestamp() {
            newest = resp.Value
        }
    }
    if newest == nil {
        return nil, gorums.ErrIncomplete
    }
    return newest, nil
}
```

**Key Changes:**

- No longer a method on a struct
- Takes `*gorums.Responses[T]` instead of `map[uint32]T`
- Returns `(T, error)` instead of `(T, bool)`
- Uses iterator: `for resp := range responses.Results()`
- Each response has `.Value` and `.Err` fields
- Return `gorums.ErrIncomplete` instead of `false`

### Step 4: Update Call Sites

Change from Configuration methods to the methods of the generated service client.
The examples below use `client`, the generated client of your service, bound to a configuration.

**Before:**

```go
ctx := context.Background()
reply, err := cfg.Read(ctx, &ReadRequest{Key: "x"})
```

**After:**

```go
// Option 1: With custom aggregation
ctx := context.Background()
client := NewStorageClient(config)
reply, err := newestState(client.ReadQC(ctx, &ReadRequest{Key: "x"}).Responses)

// Option 2: With terminal method
reply, err := client.ReadQC(ctx, &ReadRequest{Key: "x"}).Majority()

// Option 3: With threshold (e.g., wait for 3 responses)
reply, err := client.ReadQC(ctx, &ReadRequest{Key: "x"}).Threshold(3)

// Option 4: First successful response
reply, err := client.ReadQC(ctx, &ReadRequest{Key: "x"}).First()

// Option 5: Wait for all responses
reply, err := client.ReadQC(ctx, &ReadRequest{Key: "x"}).All()
```

## Terminal Methods Reference

The new design provides built-in aggregation patterns via terminal methods on `*gorums.Responses[T]`:

| Method          | Description               | Returns When                 |
| --------------- | ------------------------- | ---------------------------- |
| `.First()`      | First successful response | Any node responds            |
| `.Majority()`   | Majority of responses     | ⌈(n+1)/2⌉ responses received |
| `.All()`        | All responses             | All n nodes responded        |
| `.Threshold(n)` | At least n responses      | n responses received         |

### Example Usage

```go
// Fast reads: return first successful response
reply, err := client.ReadQC(ctx, req).First()

// Byzantine fault tolerance: require majority
reply, err := client.WriteQC(ctx, req).Majority()

// Wait for all nodes (e.g., for debugging)
reply, err := client.ReadQC(ctx, req).All()

// Custom threshold (e.g., f+1 for crash tolerance)
reply, err := client.ReadQC(ctx, req).Threshold(f + 1)
```

**Error Handling:**

Terminal methods return `gorums.ErrIncomplete` when:

- Context is canceled before threshold is met
- Not enough successful responses received
- All nodes returned errors

## Iterator-Based Custom Aggregation

For complex aggregation logic, use the iterator API with `responses.Results()`.

### Basic Iterator Pattern

```go
func customAggregation(responses *gorums.Responses[*Response]) (*Response, error) {
    for resp := range responses.Results() {
        // resp.Value contains the response message
        // resp.Err contains any error from that node
        // resp.NodeID contains the node identifier

        if resp.Err != nil {
            // Handle or skip error
            continue
        }

        // Process resp.Value
    }
    return result, nil
}
```

### Iterator Helpers

Gorums provides helper methods for common iterator operations:

#### `IgnoreErrors()` - Skip Failed Responses

```go
func countSuccessful(responses *gorums.Responses[*Response]) int {
    count := 0
    for resp := range responses.Results().IgnoreErrors() {
        count++
        // resp.Value is guaranteed to be non-nil
        // resp.Err is guaranteed to be nil
    }
    return count
}
```

To skip responses on other conditions, use an `if` statement in the loop.
To collect the responses into a map, range over the sequence and add each successful response by its `NodeID`.
To stop after a fixed number of successful responses, use the `Reduce` terminal method.

### Complete Example: Quorum with Verification

```go
func verifiedQuorum(responses *gorums.Responses[*SignedResponse]) (*SignedResponse, error) {
    const quorumSize = 3
    var verified []*SignedResponse

    for resp := range responses.Results() {
        if resp.Err != nil {
            continue
        }

        // Custom validation logic
        if !verifySignature(resp.Value) {
            continue
        }

        verified = append(verified, resp.Value)

        // Early exit once quorum is reached
        if len(verified) >= quorumSize {
            return verified[0], nil
        }
    }

    if len(verified) < quorumSize {
        return nil, gorums.ErrIncomplete
    }
    return verified[0], nil
}
```

## Async Calls

An async call sends its requests at once and collects the replies later.
`Async()` sends the requests and returns the dispatched responses; any terminal method then collects the replies.

### Basic Async Pattern

**Before:**

```go
future := cfg.ReadAsync(ctx, req)
// Do other work...
reply, err := future.Get()
```

**After:**

```go
// Send the requests now
responses := client.ReadQC(ctx, req).Async()
// Do other work...
reply, err := responses.Majority()
```

### Async with Any Terminal Method

Every terminal method works after `Async`:

```go
reply, err := client.ReadQC(ctx, req).Async().First()
reply, err = client.ReadQC(ctx, req).Async().Threshold(3)
reply, err = client.ReadQC(ctx, req).Async().Aggregate(newestValue)
```

`Async` starts no goroutine.
To wait for the replies in parallel with other work, call the terminal method in a goroutine of your own.

## Correctable Calls

Correctable calls provide progressive updates as more responses arrive.

### Basic Correctable Pattern

**Before:**

```go
correctable := cfg.ReadCorrectable(ctx, req)
reply := correctable.Get()
<-correctable.Watch(level)
reply = correctable.Get()
```

**After:**

```go
// Create correctable with initial threshold
corr := client.ReadQC(ctx, req).Correctable(2)

// Get initial result (blocks until threshold met)
reply, level, err := corr.Get()

// Watch for higher levels
<-corr.Watch(3)
reply, level, err = corr.Get()

// Or iterate through levels
for level := range corr.Levels() {
    reply, _, _ := corr.Get()
    fmt.Printf("Got reply at level %d\n", level)
}
```

### Progressive Response Handling

```go
func progressiveRead(ctx context.Context, client StorageClient, req *ReadRequest) {
    corr := client.ReadQC(ctx, req).Correctable(1)

    // Process each new level as it arrives
    for level := range corr.Levels() {
        reply, currentLevel, err := corr.Get()
        if err != nil {
            log.Printf("Error at level %d: %v", level, err)
            continue
        }

        log.Printf("Level %d: received %d replies", level, currentLevel)
        processReply(reply)

        // Stop after reaching desired level
        if currentLevel >= 3 {
            corr.Stop()
            break
        }
    }
}
```

## Custom Return Types

The new design supports aggregation functions that return types different from the proto response type.

### Example: Aggregating Multiple Responses

Consider a `StopBenchmark` RPC that returns `*MemoryStat` from each node, but you want to return `*MemoryStatList` containing all stats:

```go
// Proto definitions:
// rpc StopBenchmark(StopRequest) returns (MemoryStat)
// message MemoryStat { uint64 allocs = 1; uint64 memory = 2; }
// message MemoryStatList { repeated MemoryStat memory_stats = 1; }

// Custom aggregation function with different return type
func StopBenchmarkQF(replies map[gorums.ID]*MemoryStat) (*MemoryStatList, error) {
    if len(replies) == 0 {
        return nil, gorums.ErrIncomplete
    }
    return &MemoryStatList{
        MemoryStats: slices.Collect(maps.Values(replies)),
    }, nil
}

// Usage:
replies := collect(client.StopBenchmark(ctx, &StopRequest{}).Results())  // map[gorums.ID]*MemoryStat
memStats, err := StopBenchmarkQF(replies)  // Returns *MemoryStatList
```

### Example: Computing Aggregate Statistics

```go
// Proto: rpc StopServerBenchmark(StopRequest) returns (Result)

func StopServerBenchmarkQF(replies map[gorums.ID]*Result) (*Result, error) {
    if len(replies) == 0 {
        return nil, gorums.ErrIncomplete
    }

    // Combine results, calculating averages and pooled variance
    resp := &Result{}
    for _, reply := range replies {
        resp.TotalOps += reply.TotalOps
        resp.TotalTime += reply.TotalTime
        resp.Throughput += reply.Throughput
        resp.LatencyAvg += reply.LatencyAvg * float64(reply.TotalOps)
    }

    // Calculate weighted average
    resp.LatencyAvg /= float64(resp.TotalOps)

    // Calculate pooled variance
    for _, reply := range replies {
        resp.LatencyVar += float64(reply.TotalOps-1) * reply.LatencyVar
    }
    resp.LatencyVar /= (float64(resp.TotalOps) - float64(len(replies)))

    // Calculate averages
    numNodes := len(replies)
    resp.TotalOps /= uint64(numNodes)
    resp.TotalTime /= int64(numNodes)
    resp.Throughput /= float64(numNodes)

    return resp, nil
}
```

### Pattern: Two-Step Aggregation

For custom return types, follow this pattern:

1. **Collect responses** into a map by node ID, with a small helper such as `collect` below
2. **Apply custom aggregation function** that transforms the collected map

```go
// collect gathers the successful responses into a map by node ID.
func collect[T proto.Message](seq gorums.ResponseSeq[T]) map[gorums.ID]T {
    replies := make(map[gorums.ID]T)
    for resp := range seq.IgnoreErrors() {
        replies[resp.NodeID] = resp.Value
    }
    return replies
}

// Step 1: Collect responses
replies := collect(client.CustomQC(ctx, req).Results())  // map[gorums.ID]*ProtoResponse

// Step 2: Apply custom aggregation
result, err := CustomAggregationQF(replies)  // Returns *CustomType
```

## Configuration Manipulation

Configuration manipulation APIs remain largely unchanged:

```go
// Combine configurations
cfg3, _ := gorums.NewConfiguration(mgr, cfg1.And(cfg2))

// Exclude nodes
cfg4, _ := gorums.NewConfiguration(mgr, cfg1.Except(cfg2))

// Remove specific nodes by ID
cfg5, _ := gorums.NewConfiguration(mgr, cfg1.WithoutNodes(1, 2, 3))
```

### New: Filter Out Failed Nodes

The new design adds `WithoutErrors()` to create configurations excluding nodes that failed:

```go
// Make a quorum call
reply, err := client.ReadQC(ctx, req).Majority()

// If there was a quorum error, filter out failed nodes
if qcErr, ok := err.(gorums.QuorumCallError); ok {
    // Create new configuration without failed nodes
    goodCfg, _ := gorums.NewConfiguration(mgr, config.WithoutErrors(qcErr))

    // Retry with good nodes only
    reply, err = NewStorageClient(goodCfg).ReadQC(ctx, req).Majority()
}
```

## Error Handling

Error handling is similar but with some improvements.

### QuorumCallError

```go
reply, err := client.ReadQC(ctx, req).Majority()
if err != nil {
    // Check if it's a quorum call error
    if qcErr, ok := err.(gorums.QuorumCallError); ok {
        fmt.Printf("Quorum call failed: %v\n", qcErr)
        fmt.Printf("Reason: %s\n", qcErr.Reason)

        // Access per-node errors
        for nodeID, nodeErr := range qcErr.Errors {
            fmt.Printf("Node %d error: %v\n", nodeID, nodeErr)
        }
    }
}
```

### Context Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

client := NewStorageClient(config)
reply, err := client.ReadQC(ctx, req).Majority()
if err != nil {
    if ctx.Err() != nil {
        fmt.Println("Request timed out")
    }
}
```

## Migration Checklist

- [ ] Regenerate all `.proto` files: `make genproto`
- [ ] Remove all `QuorumSpec` interface implementations
- [ ] Convert QuorumSpec methods to standalone aggregation functions
  - [ ] Change signature from `(req *Req, replies map[uint32]*Resp) (*Resp, bool)` to `(responses *gorums.Responses[*Resp]) (*Resp, error)`
  - [ ] Use iterator: `for resp := range responses.Results()`
  - [ ] Return `gorums.ErrIncomplete` instead of `false`
- [ ] Update configuration creation
  - [ ] Remove QuorumSpec from `NewConfiguration()` calls
- [ ] Update all quorum call sites
  - [ ] Replace `cfg.Read(ctx, req)` with `client.ReadQC(ctx, req).Majority()`
  - [ ] Or use custom aggregation: `customAggregation(client.ReadQC(ctx, req))`
- [ ] Update async calls to use `Async()`
  - [ ] Replace `cfg.ReadAsync(ctx, req)` with `client.ReadQC(ctx, req).Async()`, and collect the replies later with a terminal method such as `.Majority()`
- [ ] Update correctable calls to use `.Correctable(threshold)`
  - [ ] Replace `cfg.ReadCorrectable(ctx, req)` with `client.ReadQC(ctx, req).Correctable(threshold)`
- [ ] Test thoroughly with your existing test suite
- [ ] Update documentation and examples

## Common Migration Patterns

### Pattern 1: Simple Majority Quorum

**Before:**

```go
type QSpec struct{}

func (qs *QSpec) ReadQF(_ *ReadRequest, replies map[uint32]*State) (*State, bool) {
    if len(replies) <= len(cfg.Nodes())/2 {
        return nil, false
    }
    // Return first reply
    for _, reply := range replies {
        return reply, true
    }
    return nil, false
}

cfg, _ := mgr.NewConfiguration(&QSpec{}, gorums.WithNodeList(addrs))
reply, err := cfg.Read(ctx, req)
```

**After:**

```go
// No QuorumSpec needed

config, _ := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
client := NewStorageClient(config)
reply, err := client.ReadQC(ctx, req).Majority()
```

### Pattern 2: Custom Aggregation (Select Newest)

**Before:**

```go
func (qs *QSpec) ReadQF(_ *ReadRequest, replies map[uint32]*State) (*State, bool) {
    if len(replies) < qs.quorumSize {
        return nil, false
    }
    var newest *State
    for _, s := range replies {
        if newest == nil || s.Timestamp > newest.Timestamp {
            newest = s
        }
    }
    return newest, true
}
```

**After:**

```go
func newestState(responses *gorums.Responses[*State]) (*State, error) {
    var newest *State
    for resp := range responses.Results() {
        if resp.Err != nil {
            continue
        }
        if newest == nil || resp.Value.Timestamp > newest.Value.Timestamp {
            newest = resp.Value
        }
    }
    if newest == nil {
        return nil, gorums.ErrIncomplete
    }
    return newest, nil
}

// Usage
reply, err := newestState(client.ReadQC(ctx, req).Responses)
```

### Pattern 3: Counting Responses

**Before:**

```go
func (qs *QSpec) WriteQF(_ *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool) {
    if len(replies) < qs.quorumSize {
        return nil, false
    }
    count := 0
    for _, r := range replies {
        if r.Success {
            count++
        }
    }
    return &WriteResponse{Success: count >= qs.quorumSize}, true
}
```

**After:**

```go
func countSuccessful(responses *gorums.Responses[*WriteResponse]) (*WriteResponse, error) {
    count := 0
    quorumSize := responses.Size()/2 + 1

    for resp := range responses.Results().IgnoreErrors() {
        if resp.Value.Success {
            count++
        }
    }

    if count < quorumSize {
        return nil, gorums.ErrIncomplete
    }
    return &WriteResponse{Success: true}, nil
}

// Usage
reply, err := countSuccessful(client.WriteQC(ctx, req).Responses)
```

### Pattern 4: Early Termination

**Before:**

```go
func (qs *QSpec) FastReadQF(_ *ReadRequest, replies map[uint32]*State) (*State, bool) {
    if len(replies) >= 1 {
        for _, reply := range replies {
            return reply, true  // Return first reply
        }
    }
    return nil, false
}
```

**After:**

```go
// Built-in terminal method
reply, err := client.ReadQC(ctx, req).First()

// Or custom implementation
func firstValid(responses *gorums.Responses[*State]) (*State, error) {
    for resp := range responses.Results() {
        if resp.Err != nil {
            continue
        }
        if isValid(resp.Value) {
            return resp.Value, nil
        }
    }
    return nil, gorums.ErrIncomplete
}
```

## Troubleshooting

### "Cannot use cfg.Read: undefined"

**Problem:** Configuration no longer has RPC methods.

**Solution:** Use the methods of the generated service client:

```go
client := NewStorageClient(config)
reply, err := client.ReadQC(ctx, req).Majority()
```

### "QuorumSpec interface not found"

**Problem:** QuorumSpec interface no longer generated.

**Solution:** Remove QuorumSpec implementation and convert methods to standalone functions.

### "responses.Results() not working as expected"

**Problem:** Iterator might be exhausted if used multiple times.

**Solution:** Iterators can only be consumed once. Collect into a map if you need multiple passes:

```go
replies := make(map[gorums.ID]*Response)
for resp := range responses.Results().IgnoreErrors() {
    replies[resp.NodeID] = resp.Value
}
// Now you can iterate over replies multiple times
```

### "How do I wait for all responses?"

**Solution:** Use `.All()` terminal method:

```go
reply, err := client.ReadQC(ctx, req).All()
```

### "How do I implement early termination?"

**Solution:** Return from your aggregation function as soon as the condition is met:

```go
func earlyTermination(responses *gorums.Responses[*Response]) (*Response, error) {
    for resp := range responses.Results() {
        if resp.Err == nil && condition(resp.Value) {
            return resp.Value, nil  // Early return
        }
    }
    return nil, gorums.ErrIncomplete
}
```

## Function API to Service Clients

This section applies to code that uses the generated call functions.
Those functions took a `*gorums.ConfigContext` or a `*gorums.NodeContext` as their first argument.
Gorums no longer generates them.
For each service `S`, Gorums now generates up to two client interfaces:

- `SClient` has the quorum call, streaming quorum call, and multicast methods.
  `NewSClient(cfg)` builds it from a `gorums.Config`, and its calls run on that configuration.
- `SNodeClient` has the remote call and unicast methods.
  `NewSNodeClient(node)` builds it from a `*gorums.Node`, and its calls run on that node.

A service with only one kind of call gets only that client.
Every method takes a `context.Context` and the request.

| Removed                                          | Replacement                                                |
| ------------------------------------------------ | ---------------------------------------------------------- |
| `gorums.ConfigContext` and `Config.Context`      | `NewSClient(cfg)`, then pass `ctx` to the call method      |
| `gorums.NodeContext` and `Node.Context`          | `NewSNodeClient(node)`, then pass `ctx` to the call method |
| `pb.ReadQC(cfgCtx, req)`                         | `pb.NewStorageClient(cfg).ReadQC(ctx, req)`                |
| `pb.ReadRPC(nodeCtx, req)`                       | `pb.NewStorageNodeClient(node).ReadRPC(ctx, req)`          |
| Generated `ConfigContext`, `NodeContext` aliases | None; use `context.Context`                                |

Configuration calls migrate from generated functions and a configuration context to a configuration client:

```go
// Before
cfgCtx := cfg.Context(ctx)
resp, err := pb.ReadQC(cfgCtx, req).Majority()

// After
client := pb.NewStorageClient(cfg)
resp, err := client.ReadQC(ctx, req).Majority()
```

Node calls migrate from generated functions and a node context to a node client:

```go
// Before
nodeCtx := node.Context(ctx)
resp, err := pb.ReadRPC(nodeCtx, req)

// After
resp, err := pb.NewStorageNodeClient(node).ReadRPC(ctx, req)
```

Build a node client where you need it, and do not keep it.
Building one allocates nothing.
A node client accepts any node, also a node that is not in a configuration of yours.

Configuration transformations remain on `gorums.Config`.
To call a smaller configuration, build a client for it:

```go
smaller := cfg.Remove(nodeID)
smallerClient := pb.NewStorageClient(smaller)
```

A nested call in a server handler builds a client from the peer configuration:

```go
// Before
resp, err := pb.ReadQC(ctx.PeerConfig().Context(ctx), req).Majority()

// After
resp, err := pb.NewStorageClient(ctx.PeerConfig()).ReadQC(ctx, req).Majority()
```

A generated client does not own or close its configuration or its node.
Connection ownership and the close function returned by `NewConfig` are unchanged.

A quorum call or multicast on an empty configuration still panics.
The panic now comes from the call method, not from `Config.Context`.
A remote call or unicast on a nil node no longer panics.
It returns `gorums.ErrNoNode`, a gRPC status error with the `InvalidArgument` code.
Thus, `pb.NewStorageNodeClient(cfg.Node(id))` is safe when `cfg` has no node with that ID.

Regeneration reports an error for a message, enum, enum value, or oneof wrapper type, including a nested one, whose Go name is the name of a generated client interface, constructor, or struct.
These names are `SClient`, `NewSClient`, and `sClient` if the service has quorum calls or multicasts, and `SNodeClient`, `NewSNodeClient`, and `sNodeClient` if it has remote calls or unicasts.
An RPC method may have any name.

Hand-written Go code in the generated package that declares one of these names, such as your own `StorageClient` wrapper, now fails to compile.
The generator cannot detect this collision, so rename such code.

## Additional Resources

- [User Guide](user-guide.md) - Complete guide to using Gorums
- [Developer Guide](dev-guide.md) - Contributing to Gorums

- [Examples](../examples/) - Working examples demonstrating various patterns

## Summary

The v3 design provides more flexibility and better Go idioms:

**Key Improvements:**

- ✅ No more QuorumSpec interface boilerplate
- ✅ Built-in terminal methods for common patterns
- ✅ Flexible iterator-based aggregation
- ✅ Progressive response handling with correctables
- ✅ Custom return types support
- ✅ Better error filtering with `WithoutErrors()`

**Migration Effort:**

- Low: For code using simple majority/first patterns (switch to terminal methods)
- Medium: For code with custom QuorumSpec logic (convert to iterators)
- Benefits: Cleaner code, more flexibility, better performance

Welcome to Gorums v3! 🎉
