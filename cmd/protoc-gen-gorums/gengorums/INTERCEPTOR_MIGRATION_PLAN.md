# Interceptor-Based QuorumCall Migration Plan

## Overview

This document outlines the plan for migrating from the legacy `QuorumCall` implementation to the new interceptor-based approach. The new approach provides more flexibility, composability, and cleaner separation of concerns.

## Key Design Decisions

The following breaking changes will be made as part of this migration:

1. **Remove `gorums.per_node_arg` option** — Per-node transformation will be provided via an optional `WithTransformFunc` call option parameter instead of a proto option.

2. **Remove `gorums.custom_return_type` option** — The `Out` type parameter can be any type (not restricted to `proto.Message`), giving users full flexibility in their quorum functions.

3. **New per-node transform signature** — Changes from `func(*Request, uint32)` (node ID) to `func(*Request, *gorums.RawNode)` (full node access).

4. **Defer async migration** — Focus on synchronous quorum calls first to reduce risk.

## Current State

### Legacy Approach

The current implementation uses `QuorumCall` with `QuorumCallData`:

- `QuorumCallData.PerNodeArgFn`: `func(proto.Message, uint32) proto.Message` — per-node request transformation
- `QuorumCallData.QuorumFunction`: `func(proto.Message, map[uint32]proto.Message) (proto.Message, bool)` — response aggregation
- Proto options: `gorums.per_node_arg`, `gorums.custom_return_type`
- Type erasure via `proto.Message` (no generics)
- Tightly coupled to QuorumSpec interface

### New Interceptor-Based Approach

The new implementation uses `QuorumCallWithInterceptor`:

- **Type Safety**: Full generics with `[Req, Resp msg, Out any]` type parameters
- **Flexible Output**: `Out` can be any type, not just `proto.Message`
- **Per-Node Transform**: Via `WithTransformFunc` call option or `PerNodeTransform` interceptor
- **Composability**: Chain multiple interceptors for complex behavior
- **Lazy Sending**: Messages sent only when `Responses()` is called, allowing transform registration

## Built-In Components

### Base Quorum Functions (in `client_interceptor.go`)

| Function                         | Signature                                | Purpose                          |
| -------------------------------- | ---------------------------------------- | -------------------------------- |
| `ThresholdQuorum[Req, Resp](n)`  | `QuorumFunc[Req, Resp, Resp]`            | Wait for n successful responses  |
| `MajorityQuorum[Req, Resp]`      | `QuorumFunc[Req, Resp, Resp]`            | Wait for ⌈(n+1)/2⌉ responses     |
| `FirstResponse[Req, Resp]`       | `QuorumFunc[Req, Resp, Resp]`            | Return first successful response |
| `AllResponses[Req, Resp]`        | `QuorumFunc[Req, Resp, Resp]`            | Wait for all nodes               |
| `CollectAllResponses[Req, Resp]` | `QuorumFunc[Req, Resp, map[uint32]Resp]` | Collect all into map             |

### Interceptors

| Interceptor                                 | Purpose                                       |
| ------------------------------------------- | --------------------------------------------- |
| `PerNodeTransform[Req, Resp, Out](fn)`      | Apply per-node request transformations        |
| `QuorumSpecInterceptor[Req, Resp, Out](qf)` | Adapter for legacy QuorumSpec-style functions |

### Iterator Helpers (methods on `Responses[T]`)

| Method              | Purpose                    |
| ------------------- | -------------------------- |
| `IgnoreErrors()`    | Filter out error responses |
| `Filter(keep func)` | Custom filtering           |
| `CollectN(n)`       | Collect up to n responses  |
| `CollectAll()`      | Collect all responses      |

## Migration Strategy

### Phase 1: Coexistence (COMPLETED ✅)

- Legacy `QuorumCall` kept for backward compatibility during migration
- Interceptor infrastructure implemented alongside legacy code

### Phase 2: Template Updates (CURRENT)

This phase involves updating the code generation templates.

#### Step 2.1: Remove deprecated proto options

**File: `gorums.proto`**

Remove these options:
```proto
bool per_node_arg         = 50020;  // REMOVE
string custom_return_type = 50030;  // REMOVE
```

#### Step 2.2: Update template helper functions

**File: `gengorums/gorums_func_map.go`**

Remove:
- `hasPerNodeArg` function
- `perNodeArg` function
- `perNodeFnType` function
- `customOut` function (or simplify to always return `out`)

#### Step 2.3: Update QuorumSpec template

**File: `gengorums/template_qspec.go`**

Change the QuorumSpec interface signature from:
```go
MethodQF(in *Request, replies map[uint32]*Response) (*CustomOut, bool)
```

To:
```go
MethodQF(in *Request, replies map[uint32]*Response) (*Response, bool)
```

The return type always matches the RPC response type. Users who need different output types should implement custom interceptors.

#### Step 2.4: Update quorum call template

**File: `gengorums/template_quorumcall.go`**

Replace `quorumCallBody` to use `QuorumCallWithInterceptor`:

```go
var quorumCallBody = `
	return {{$quorumCallWithInterceptor}}(
		ctx, c.RawConfiguration, in, "{{$fullName}}",
		{{$quorumSpecInterceptor}}(c.qspec.{{$method}}QF),
		opts...,
	)
}
`
```

The new signature will be:
```go
func (c *Configuration) Method(
    ctx context.Context,
    in *Request,
    opts ...gorums.QuorumCallOption,  // NEW: variadic options for transforms, etc.
) (*Response, error)
```

#### Step 2.5: Add QuorumCallOption type

**File: `client_interceptor.go`**

Add a new option type for quorum calls:

```go
// QuorumCallOption configures a quorum call.
type QuorumCallOption interface {
    apply(*quorumCallOptions)
}

// WithTransformFunc returns an option that applies per-node request transformations.
// The transform function receives the original request and a node, and returns
// the transformed request to send to that node.
func WithTransformFunc[Req msg](fn func(Req, *RawNode) Req) QuorumCallOption {
    // Implementation
}
```

#### Step 2.6: Update multicast/unicast templates

Similar updates for multicast and unicast templates to use the new option-based transform.

### Phase 3: Update test protos and examples

#### Files to update:

1. `cmd/protoc-gen-gorums/dev/zorums.proto` — Remove `per_node_arg` and `custom_return_type` options
2. `internal/testprotos/calltypes/zorums/zorums.proto` — Same updates
3. `internal/tests/oneway/oneway.proto` — Remove `per_node_arg` option
4. `benchmark/benchmark.proto` — Remove `custom_return_type` option
5. `examples/storage/proto/storage.proto` — Check and update if needed

#### Test files to update:

All tests using `per_node_arg` signature need to change from:
```go
f := func(req *Request, nodeID uint32) *Request { ... }
config.Method(ctx, req, f)
```

To:
```go
f := func(req *Request, node *gorums.RawNode) *Request { ... }
config.Method(ctx, req, gorums.WithTransformFunc(f))
```

### Phase 4: Cleanup

- Remove legacy `QuorumCall` and `QuorumCallData` from `quorumcall.go`
- Remove `E_PerNodeArg` and `E_CustomReturnType` extension handling
- Update all documentation

### Phase 5: Async Migration (DEFERRED)

- Migrate `AsyncCall` to interceptor-based approach
- This will be done in a subsequent phase to reduce risk

## Generated Code Examples

### Before (Legacy)

```go
// QuorumSpec interface
type QuorumSpec interface {
    gorums.ConfigOption
    MethodQF(in *Request, replies map[uint32]*Response) (*CustomOut, bool)
    MethodPerNodeArgQF(in *Request, replies map[uint32]*Response) (*Response, bool)
}

// Generated quorum call (plain)
func (c *Configuration) Method(ctx context.Context, in *Request) (*Response, error) {
    cd := gorums.QuorumCallData{Message: in, Method: "..."}
    cd.QuorumFunction = func(req proto.Message, replies map[uint32]proto.Message) (proto.Message, bool) {
        r := make(map[uint32]*Response, len(replies))
        for k, v := range replies { r[k] = v.(*Response) }
        return c.qspec.MethodQF(req.(*Request), r)
    }
    res, err := c.RawConfiguration.QuorumCall(ctx, cd)
    return res.(*Response), err
}

// Generated quorum call (with per_node_arg)
func (c *Configuration) MethodPerNodeArg(ctx context.Context, in *Request,
    f func(*Request, uint32) *Request) (*Response, error) {
    cd := gorums.QuorumCallData{...}
    cd.PerNodeArgFn = func(req proto.Message, nid uint32) proto.Message {
        return f(req.(*Request), nid)
    }
    // ...
}
```

### After (Interceptor-Based)

```go
// QuorumSpec interface (simplified)
type QuorumSpec interface {
    gorums.ConfigOption
    MethodQF(in *Request, replies map[uint32]*Response) (*Response, bool)
}

// Generated quorum call (all methods have same signature)
func (c *Configuration) Method(ctx context.Context, in *Request,
    opts ...gorums.QuorumCallOption) (*Response, error) {
    return gorums.QuorumCallWithInterceptor(
        ctx, c.RawConfiguration, in, "service.Method",
        gorums.QuorumSpecInterceptor(c.qspec.MethodQF),
        opts...,
    )
}

// Usage with per-node transform
resp, err := config.Method(ctx, req, gorums.WithTransformFunc(
    func(req *Request, node *gorums.RawNode) *Request {
        return &Request{Value: fmt.Sprintf("%s-%d", req.Value, node.ID())}
    },
))
```

## Implementation Tasks

### Completed ✅

- [x] Implement core interceptor infrastructure (`client_interceptor.go`)
- [x] Add built-in base quorum functions (MajorityQuorum, FirstResponse, etc.)
- [x] Add `PerNodeTransform` interceptor
- [x] Add `QuorumSpecInterceptor` adapter
- [x] Write comprehensive tests (17 test functions)
- [x] Implement lazy sending with `RegisterTransformFunc` and `sendOnce`

### Phase 2 Tasks (Current Sprint)

- [ ] Add `QuorumCallOption` type and `WithTransformFunc` option
- [ ] Update `QuorumCallWithInterceptor` to accept options
- [ ] Remove `per_node_arg` and `custom_return_type` from `gorums.proto`
- [ ] Regenerate `gorums.pb.go`
- [ ] Update `gorums_func_map.go` — remove deprecated helper functions
- [ ] Update `template_qspec.go` — simplify QuorumSpec interface
- [ ] Update `template_quorumcall.go` — use interceptor-based approach
- [ ] Update `template_multicast.go` — use option-based transform
- [ ] Update `template_unicast.go` — use option-based transform
- [ ] Regenerate all `zorums_*_gorums.pb.go` files
- [ ] Update test protos (remove deprecated options)
- [ ] Update test implementations (new transform signature)
- [ ] Run `make test` to validate

### Phase 3 Tasks (Documentation)

- [ ] Update `doc/method-options.md`
- [ ] Update `doc/user-guide.md`
- [ ] Create migration guide for users
- [ ] Update examples

### Phase 4 Tasks (Cleanup)

- [ ] Remove legacy `QuorumCall` and `QuorumCallData`
- [ ] Remove deprecated extension handling code
- [ ] Benchmark and optimize

### Phase 5 Tasks (Deferred)

- [ ] Migrate async quorum calls to interceptor-based approach

## Design Decisions (Resolved)

1. **QuorumCallOption implementation**: Use type erasure internally (`func(proto.Message, *RawNode) proto.Message`) for simpler generated code. Users still get compile-time type checks at call sites.

2. **Correctable calls**: Defer to Phase 5 with async, since it has similar complexity.

3. **Multicast/Unicast transform support**: Yes, support `WithTransformFunc` for consistency.

4. **QuorumSpec interface changes**: Always return the RPC response type. Users needing custom outputs write custom interceptors.

5. **Transform function receives original request**: Pass original (not clone) for performance. User responsibility to clone if needed.

6. **Backward compatibility**: No deprecation period. This is a breaking change requiring major version bump.

## Notes

- The `Out` type parameter is `any`, allowing quorum functions to return any type
- `Req` and `Resp` are still constrained to `proto.Message` (type alias `msg`)
- Lazy sending allows interceptors to register transforms before messages are sent
- Multiple transforms can be chained via `RegisterTransformFunc`
- The new approach is more composable and testable
