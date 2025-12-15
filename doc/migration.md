# Migration Guide: Gorums v2 to v3

This guide helps you migrate from Gorums v2 (QuorumSpec-based design) to v3 (iterator-based design with generic functions).

## Overview of Changes

Gorums v3 introduces a fundamental redesign focused on:

- **Type aliases** instead of generated concrete types
- **Generic functions** with terminal methods instead of QuorumSpec interface
- **Iterator-based response aggregation** for flexible custom logic
- **ConfigContext pattern** for passing configuration to generic functions
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
    for resp := range responses.Seq() {
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

// Generic function with ConfigContext
cfgCtx := config.Context(ctx)
responses := ReadQC(cfgCtx, &ReadRequest{})

// Option 1: Use custom aggregation
reply, err := newestValue(responses)

// Option 2: Use built-in terminal method
reply, err := ReadQC(cfgCtx, &ReadRequest{}).Majority()
```

## Migration Steps

### Step 1: Regenerate Protocol Buffers

Regenerate all `.proto` files with the new `protoc-gen-gorums` plugin:

```bash
make genproto
```

This generates new `*_gorums.pb.go` files with:

- Type aliases for `Manager`, `Configuration`, `Node`
- Generic functions for quorum calls (e.g., `ReadQC`, `WriteQC`)
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
    for resp := range responses.Seq() {
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
- Uses iterator: `for resp := range responses.Seq()`
- Each response has `.Value` and `.Err` fields
- Return `gorums.ErrIncomplete` instead of `false`

### Step 4: Update Call Sites

Change from Configuration methods to generic functions with ConfigContext.

**Before:**

```go
ctx := context.Background()
reply, err := cfg.Read(ctx, &ReadRequest{Key: "x"})
```

**After:**

```go
// Option 1: With custom aggregation
ctx := context.Background()
cfgCtx := config.Context(ctx)
reply, err := newestState(ReadQC(cfgCtx, &ReadRequest{Key: "x"}))

// Option 2: With terminal method
reply, err := ReadQC(cfgCtx, &ReadRequest{Key: "x"}).Majority()

// Option 3: With threshold (e.g., wait for 3 responses)
reply, err := ReadQC(cfgCtx, &ReadRequest{Key: "x"}).Threshold(3)

// Option 4: First successful response
reply, err := ReadQC(cfgCtx, &ReadRequest{Key: "x"}).First()

// Option 5: Wait for all responses
reply, err := ReadQC(cfgCtx, &ReadRequest{Key: "x"}).All()
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
reply, err := ReadQC(cfgCtx, req).First()

// Byzantine fault tolerance: require majority
reply, err := WriteQC(cfgCtx, req).Majority()

// Wait for all nodes (e.g., for debugging)
reply, err := ReadQC(cfgCtx, req).All()

// Custom threshold (e.g., f+1 for crash tolerance)
reply, err := ReadQC(cfgCtx, req).Threshold(f + 1)
```

**Error Handling:**

Terminal methods return `gorums.ErrIncomplete` when:

- Context is canceled before threshold is met
- Not enough successful responses received
- All nodes returned errors

## Iterator-Based Custom Aggregation

For complex aggregation logic, use the iterator API with `responses.Seq()`.

### Basic Iterator Pattern

```go
func customAggregation(responses *gorums.Responses[*Response]) (*Response, error) {
    for resp := range responses.Seq() {
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
    for resp := range responses.Seq().IgnoreErrors() {
        count++
        // resp.Value is guaranteed to be non-nil
        // resp.Err is guaranteed to be nil
    }
    return count
}
```

#### `Filter()` - Custom Filtering

```go
func newestValue(responses *gorums.Responses[*ReadResponse]) (*ReadResponse, error) {
    var newest *ReadResponse

    // Filter for responses with non-zero timestamps
    filtered := responses.Seq().Filter(func(nr gorums.NodeResponse[*ReadResponse]) bool {
        return nr.Err == nil && nr.Value.GetTimestamp() > 0
    })

    for resp := range filtered {
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

#### `CollectN()` - Collect First N Responses

```go
func majorityWrite(responses *gorums.Responses[*WriteResponse]) (*WriteResponse, error) {
    majority := (responses.Size() + 1) / 2
    replies := responses.Seq().IgnoreErrors().CollectN(majority)

    if len(replies) < majority {
        return nil, gorums.ErrIncomplete
    }

    // Process the map[uint32]*WriteResponse
    return aggregateWrites(replies), nil
}
```

#### `CollectAll()` - Collect All Responses

```go
// Collect all responses into a map
replies := responses.Seq().CollectAll()
// Returns map[uint32]*Response (may include failed nodes with nil values)

// Collect only successful responses
replies := responses.Seq().IgnoreErrors().CollectAll()
// Returns map[uint32]*Response (only successful responses)
```

### Complete Example: Quorum with Verification

```go
func verifiedQuorum(responses *gorums.Responses[*SignedResponse]) (*SignedResponse, error) {
    const quorumSize = 3
    var verified []*SignedResponse

    for resp := range responses.Seq() {
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

Async calls remain similar but now use terminal methods to create futures.

### Basic Async Pattern

**Before:**

```go
future := cfg.ReadAsync(ctx, req)
// Do other work...
reply, err := future.Get()
```

**After:**

```go
// Create async future with terminal method
future := ReadQC(cfgCtx, req).AsyncMajority()
// Do other work...
reply, err := future.Get()
```

### Async Terminal Methods

All terminal methods have async variants:

```go
// Async majority
fut := ReadQC(cfgCtx, req).AsyncMajority()

// Async first
fut := ReadQC(cfgCtx, req).AsyncFirst()

// Async all
fut := ReadQC(cfgCtx, req).AsyncAll()

// Async threshold
fut := ReadQC(cfgCtx, req).AsyncThreshold(3)
```

### Using Async with Custom Aggregation

For custom aggregation with async, get responses first then apply aggregation:

```go
// Start the quorum call
responses := ReadQC(cfgCtx, req)

// Get async future for custom threshold
future := responses.AsyncThreshold(3)

// Do other work...

// Wait for completion
reply, err := future.Get()
if err != nil {
    return err
}

// Now apply custom aggregation to collected responses
result := customAggregation(reply)
```

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
corr := ReadQC(cfgCtx, req).Correctable(2)

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
func progressiveRead(cfgCtx *gorums.ConfigContext, req *ReadRequest) {
    corr := ReadQC(cfgCtx, req).Correctable(1)

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
func StopBenchmarkQF(replies map[uint32]*MemoryStat) (*MemoryStatList, error) {
    if len(replies) == 0 {
        return nil, gorums.ErrIncomplete
    }
    return &MemoryStatList{
        MemoryStats: slices.Collect(maps.Values(replies)),
    }, nil
}

// Usage:
responses := StopBenchmark(cfgCtx, &StopRequest{})
replies := responses.Seq().IgnoreErrors().CollectAll()  // map[uint32]*MemoryStat
memStats, err := StopBenchmarkQF(replies)  // Returns *MemoryStatList
```

### Example: Computing Aggregate Statistics

```go
// Proto: rpc StopServerBenchmark(StopRequest) returns (Result)

func StopServerBenchmarkQF(replies map[uint32]*Result) (*Result, error) {
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

1. **Collect responses** using `CollectAll()` or `CollectN()`
2. **Apply custom aggregation function** that transforms the collected map

```go
// Step 1: Collect responses
responses := CustomQC(cfgCtx, req)
replies := responses.Seq().IgnoreErrors().CollectAll()  // map[uint32]*ProtoResponse

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
reply, err := ReadQC(cfgCtx, req).Majority()

// If there was a quorum error, filter out failed nodes
if qcErr, ok := err.(gorums.QuorumCallError); ok {
    // Create new configuration without failed nodes
    goodCfg, _ := gorums.NewConfiguration(mgr, config.WithoutErrors(qcErr))

    // Retry with good nodes only
    cfgCtx := goodCfg.Context(ctx)
    reply, err = ReadQC(cfgCtx, req).Majority()
}
```

## Error Handling

Error handling is similar but with some improvements.

### QuorumCallError

```go
reply, err := ReadQC(cfgCtx, req).Majority()
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

cfgCtx := config.Context(ctx)
reply, err := ReadQC(cfgCtx, req).Majority()
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
  - [ ] Use iterator: `for resp := range responses.Seq()`
  - [ ] Return `gorums.ErrIncomplete` instead of `false`
- [ ] Update configuration creation
  - [ ] Remove QuorumSpec from `NewConfiguration()` calls
- [ ] Update all quorum call sites
  - [ ] Replace `cfg.Read(ctx, req)` with `ReadQC(cfgCtx, req).Majority()`
  - [ ] Or use custom aggregation: `customAggregation(ReadQC(cfgCtx, req))`
- [ ] Update async calls to use async terminal methods
  - [ ] Replace `cfg.ReadAsync(ctx, req)` with `ReadQC(cfgCtx, req).AsyncMajority()`
- [ ] Update correctable calls to use `.Correctable(threshold)`
  - [ ] Replace `cfg.ReadCorrectable(ctx, req)` with `ReadQC(cfgCtx, req).Correctable(threshold)`
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
cfgCtx := config.Context(ctx)
reply, err := ReadQC(cfgCtx, req).Majority()
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
    for resp := range responses.Seq() {
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
reply, err := newestState(ReadQC(cfgCtx, req))
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
    quorumSize := (responses.Size() + 1) / 2

    for resp := range responses.Seq().IgnoreErrors() {
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
reply, err := countSuccessful(WriteQC(cfgCtx, req))
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
reply, err := ReadQC(cfgCtx, req).First()

// Or custom implementation
func firstValid(responses *gorums.Responses[*State]) (*State, error) {
    for resp := range responses.Seq() {
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

**Solution:** Use generic functions with ConfigContext:

```go
cfgCtx := config.Context(ctx)
reply, err := ReadQC(cfgCtx, req).Majority()
```

### "QuorumSpec interface not found"

**Problem:** QuorumSpec interface no longer generated.

**Solution:** Remove QuorumSpec implementation and convert methods to standalone functions.

### "responses.Seq() not working as expected"

**Problem:** Iterator might be exhausted if used multiple times.

**Solution:** Iterators can only be consumed once. Collect into a map if you need multiple passes:

```go
replies := responses.Seq().CollectAll()
// Now you can iterate over replies multiple times
```

### "How do I wait for all responses?"

**Solution:** Use `.All()` terminal method:

```go
reply, err := ReadQC(cfgCtx, req).All()
```

### "How do I implement early termination?"

**Solution:** Return from your aggregation function as soon as the condition is met:

```go
func earlyTermination(responses *gorums.Responses[*Response]) (*Response, error) {
    for resp := range responses.Seq() {
        if resp.Err == nil && condition(resp.Value) {
            return resp.Value, nil  // Early return
        }
    }
    return nil, gorums.ErrIncomplete
}
```

## Additional Resources

- [User Guide](user-guide.md) - Complete guide to using Gorums
- [Developer Guide](dev-guide.md) - Contributing to Gorums
- [Method Options](method-options.md) - Protocol buffer options reference
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
