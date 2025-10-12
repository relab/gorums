# Channel Redesign Plan

I want to redesign the #file:channel.go implementation, which is unnecessarily complex.

I'm wondering if the logic in #file:channel.go and the gRPC logic in #file:node.go can be simplified using gRPC APIs instead of our own custom logic. The main reason for our custom logic is that we need to invoke different servers concurrently to implement our multicast and quorumcall logic. By concurrent, I mean without having to wait for the reply before invoking the next server. I think only #sym:(*channel).enqueue is used by the #file:multicast.go and #file:quorumcall.go and other similar call types. How would you redesign the channel?

---

To get rid of the fragile “home‑grown stream + router” machinery while still supporting true concurrent fan‑out for multicast and quorum calls, I’d split the redesign into three layers:

Per‑node transport: drop the persistent bidirectional stream and instead let each request use its own short‑lived ClientConn. Keep one ClientConn per node, but create lightweight RPC contexts per request. That puts flow‑control, retries, and transport errors back in gRPC’s hands. In practice, use grpc.ClientConn.Invoke (unary) or a generated stub per handler; concurrency becomes a non‑issue because gRPC is already threadsafe.

Request tracker: replace the current router map with a simple “in‑flight” structure keyed by message ID that holds contexts and cancellation functions. Each send spawns a goroutine that performs Invoke and pushes the result to the caller’s channel. When the goroutine finishes, it cleans itself up; no special waitForSend() branch is needed because returning early is just skipping the result wait.

Fan‑out orchestration: make enqueue essentially “submit work to the per‑node worker pool.” For multicast/quorum, construct the message, call node.transport.Send(ctx, message, opts) where Send just schedules a goroutine that invokes the RPC. Responses land in the reply channel directly, with gRPC’s own errors (deadline, unavailable, etc.) propagated, so the higher layers don’t see the synthetic “stream is down” placeholder.

This keeps the API surface you already have (enqueue, message IDs) but delegates failure handling and re‑dials to gRPC. The only custom logic left is batching replies and enforcing quorum semantics at the configuration layer.

---

We used to implement per-node transport using unary RPCs, but there was a subtle bug caused by two (or more) servers delivering invocations in different orders, even though the single client invoked them in-order. This issue has more information: <https://github.com/relab/gorums/issues/16>

This is why we introduced streaming RPC, which allowed us to control FIFO delivery from the same client.

Will your suggested design work with these considerations?

---

Good reminder—that GitHub issue was the driver for switching to a long-lived stream, because gRPC’s default client stack can issue multiple unary calls in parallel and the server may process them in any order. If FIFO per client per node is a hard requirement, a naïve “spawn a unary RPC per message” design falls straight back into that bug.

You still can simplify, but you need to keep one serialization point per node. Two ways to retain FIFO without the brittle pieces we have today:

Streaming but leaner. Keep a single NodeStream per node, but move send/recv off the shared router map. Have one dedicated send goroutine per node that reads from sendQ and does stream.Send(); use another goroutine that only reads from stream.Recv() and forwards responses. When the stream errors, tear down the pair and rebuild it synchronously so the requester waits for the new stream instead of getting streamDownErr. Context cancellation on individual requests can just drop them from the queue and let the send goroutine skip them.

Unary with per-node sequencing. If you really want to go back to unary, add a per-node worker that handles the single FIFO queue. Each worker issues one Invoke, waits for it to finish, then starts the next. That preserves order, because only one RPC per node is inflight at a time. You still get concurrency across nodes (the workers operate independently), but you don’t rely on gRPC to serialize anything for you.

Either approach keeps the FIFO guarantee and avoids the “short lived stream per message” bug. The first stays closer to the current streaming API but with safer reconnect semantics; the second drops streaming entirely yet still satisfies in-order delivery.

## Goals

- Document and evaluate two alternative per-node transport designs that preserve FIFO semantics while simplifying the channel implementation.
- Produce a regression test that ensures per-node FIFO ordering is preserved (covers the bug described in relab/gorums#16).
- Delegate prototype implementations of both alternatives to coding agents for local experimentation only (no pull requests).

## Scope & Deliverables

1. **Regression Test**
   - Add a Go test demonstrating the out-of-order delivery scenario and asserting FIFO behaviour.
   - Test should pass with the current streaming implementation and fail if FIFO is broken.

2. **Prototype A – Stream-Based Simplification**
   - Maintain long-lived streaming RPC per node.
   - Investigate removing ad-hoc router management by introducing dedicated send/receive goroutines with synchronous reconnection.
   - Prototype placed in a new file (e.g., `channel_stream_proto.go`) without replacing existing production code.

3. **Prototype B – Sequenced Unary Transport**
   - Replace bidirectional streaming with per-node worker that issues unary RPCs sequentially to maintain FIFO.
   - Prototype placed in a new file (e.g., `channel_unary_proto.go`).

4. **Evaluation notes**
   - Each prototype should include brief comments summarising approach, trade-offs, and open questions.
   - Ensure both prototypes are isolated so they do not affect current build/tests unless explicitly wired in.

5. **Delegation**
   - After the regression test lands, trigger two separate coding-agent tasks (one per prototype) with instructions to keep changes local.
   - Review agent output manually.

## Validation Strategy

- Run `go test ./...` focusing on channel-related packages after adding the FIFO regression test.
- For each prototype, compile affected packages and run targeted tests to ensure no build breakage.
- Compare behaviour against the regression test and log observations for discussion with Hein.

## Open Questions

- Should both prototypes share helper utilities (e.g., transport interface), or remain fully independent for comparison?
- How should we measure performance impact for each approach (micro-benchmarks vs. existing benchmark suite)?
- Do we need additional instrumentation to observe reconnection behaviour under load?
