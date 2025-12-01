# Review of Commit c54406e4: Quorum Call API Redesign

## Summary

The commit redesigns the quorum call API to use fluent terminal methods:

**Old API:**

```go
resp, err := ReadQC(ctx, req)
resp, err := ReadQC(ctx, req, gorums.WithQuorumFunc(myQF))
```

**New API:**

```go
resp, err := ReadQC(ctx, req).First()
resp, err := ReadQC(ctx, req).Majority()
resp, err := ReadQC(ctx, req).All()
resp, err := ReadQC(ctx, req).Threshold(n)
```

This is a significant improvement in API ergonomics, but the implementation has unnecessary complexity.

## Issue: `Responses[Req, Resp]` with Both Type Parameters

The current `Responses` struct specifies both request and response types:

```go
type Responses[Req, Resp msg] struct {
    ctx *ClientCtx[Req, Resp]
}
```

### Why It Currently Has Both `Req` and `Resp`

The `Responses` struct holds a `*ClientCtx[Req, Resp]`, and `ClientCtx` needs `Req` for:

- `Request()` method to return the original request
- `RegisterTransformFunc(fn func(Req, *RawNode) Req)` for per-node request transformation
- Interceptors (`MapRequest`, `MapResponse`, `Map`) that need both types

### Problems with Current Design

1. **User-facing API is more complex**: Users see `*gorums.Responses[*Request, *Response]` instead of just `*gorums.Responses[*Response]`
2. **The `Req` type is unnecessary for terminal methods**: `First()`, `Majority()`, `All()`, `Threshold()` only work with responses
3. **Generated code is more verbose**: The return type exposes internal complexity

### Generated Code Example

```go
func QuorumCall(ctx *gorums.ConfigContext, in *Request, opts ...gorums.CallOption) *gorums.Responses[*Request, *Response] {
    return gorums.QuorumCallWithInterceptor[*Request, *Response](...)
}
```

Users don't need to know about `*Request` when calling `.Majority()`.

## Missing Design: Generated Wrapper with Embedded `gorums.Responses[Resp]`

An earlier discussion proposed generating wrapper types:

**In gorums package:**

```go
type Responses[Resp msg] struct {
    // base implementation with terminal methods
}
```

**In generated code:**

```go
type Responses struct {
    *gorums.Responses[*Response] // embedded
}
```

This pattern was **not implemented**. Instead, the generated code directly returns `*gorums.Responses[Req, Resp]`.

## Recommended Simplification

### Goal

Change from `Responses[Req, Resp]` to `Responses[Resp]` for a cleaner user-facing API.

### Key Changes

1. **Keep `ClientCtx[Req, Resp]` internal** - It still needs both types for request transformations
2. **Create `Responses[Resp]` with only response type** - Terminal methods only need `Resp`
3. **Refactor interceptors** - They receive `*ClientCtx` (internal) and return `*Responses[Resp]`

### New Design

```go
// Responses provides access to quorum call responses and terminal methods.
// Type parameter:
//   - Resp: The response message type from individual nodes
type Responses[Resp msg] struct {
    responseSeq ResponseSeq[Resp]
    size        int
}

// QuorumInterceptor intercepts and processes quorum calls.
// It receives the internal context and returns a Responses object.
type QuorumInterceptor[Req, Resp msg] func(*ClientCtx[Req, Resp]) *Responses[Resp]
```

### Benefits

1. **Simpler user API**: `*gorums.Responses[*Response]`
2. **Cleaner generated code**: Less type noise
3. **Separation of concerns**: Internal `ClientCtx` vs user-facing `Responses`
4. **Interceptors remain powerful**: They still have access to both types

### Generated Code After Simplification

```go
func QuorumCall(ctx *gorums.ConfigContext, in *Request, opts ...gorums.CallOption) *gorums.Responses[*Response] {
    return gorums.QuorumCallWithInterceptor[*Request, *Response](...)
}
```

## Other Observations

1. **`ClientCtx` is public** - Should be unexported if only used internally
2. **Interceptor pattern changed** - Now modifies `Responses` in place vs chain-of-functions
3. **`WaitForLevel` returns `*Correctable[Resp]`** - Already uses single type parameter (good)

## Action Items

1. Simplify `Responses` to `Responses[Resp]`
2. Keep `ClientCtx[Req, Resp]` as internal implementation detail
3. Update `QuorumCallWithInterceptor` to return `*Responses[Resp]`
4. Update generated templates
5. Run all tests to verify the changes
