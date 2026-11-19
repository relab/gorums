# TLA+ Model of the Stream Layer

This model covers the Gorums stream layer for one peer pair under stream deduplication.
TLC checks it for small bounds.

* The current code passes every invariant, the deadlock check, and the liveness property `BorrowerRecovers`.
* The model finds the four past stream-layer bugs when a switch puts each one back.
* The current code runs each call at most once, and it starts the calls of each stream in FIFO order.
* A new stream is an ordering boundary: a call from an older stream can overlap with, or start after, a call from its replacement.

## What the Model Covers

D is the lower-ID peer and the only dialer.
A is the higher-ID peer.
A's outbound node for D is born shared, so A's calls to D use D's stream.
D opens one stream at a time.
A gives each accepted stream its own inbound channel.

![D's outbound channel and A's inbound channel, joined by stream s](figures/structure.svg)

D's calls go from D's send queue across the top wire to A's dispatcher, and A's replies come back on the bottom wire.
A's own calls enter A's send queue through the slot (dotted) and take the same path in the other direction.
Each queue, wire, and dispatcher has a capacity, so the model has flow control.
The dashed boxes are the pending tables: the sendLoop adds a call before it sends it, and a reply removes it.

The network can break a stream, and the frames in flight are lost.
Callers can give up, as when a context deadline expires.

| Model                                    | Code                                                                    |
| ---------------------------------------- | ----------------------------------------------------------------------- |
| `OpenD`, `TakeD`, `SendD`, `EndD`        | `OutboundChannel.run`, `session.sendLoop`, `session.end`, `failPending` |
| `RecvD`, `RecvS`                         | `session.handle`                                                        |
| `HandleD`, `ReplyD`, `HandleS`, `ReplyS` | `dispatcher`, `endpoint.reply`                                          |
| `AcceptS`, `ServerEndS`, `live`, `slot`  | `InboundManager.registerPeer`, `Node.attachStream`, `liveChannels`      |
| `IssueB`                                 | `Transport.Enqueue` on the shared transport                             |

Not modeled yet: refusals and their backoff, GOAWAY and `Drain`, streaming and one-way calls, `ResourceExhausted`, `LocalChannel`, and application `Close`.

### Switches

Each configuration sets every switch.
All switches off models the current code.

| Switch                   | Kind    | Effect                                                                                  |
| ------------------------ | ------- | --------------------------------------------------------------------------------------- |
| `LazyReconnect`          | bug     | D reopens a lost stream only when it has a request.                                     |
| `AttachClosesPrior`      | bug     | Attaching a stream closes the prior channel.                                            |
| `BlockingReplies`        | bug     | A handler's reply waits for send queue space.                                           |
| `Requeue`                | bug     | A lost stream requeues its pending calls at the tail of the send queue.                 |
| `LateFrames`             | network | A break loses only a suffix of the frames in flight. The receiver still gets the rest.  |

`LateFrames` models data that is still in transit or in buffers when the sender gives up, as after a keepalive timeout.

## Properties

| Property               | Kind      | Meaning                                                                                       |
| ---------------------- | --------- | --------------------------------------------------------------------------------------------- |
| `AtMostOnceDelivery`   | invariant | The runtime delivers at most one response to each call.                                       |
| `NoDuplicateCall`      | invariant | A call is in at most one send queue or pending table.                                         |
| `SlotTracksLive`       | invariant | While A serves a stream that is not broken, the shared node's slot holds an attached channel. |
| `OlderStreamsEnded`    | invariant | D opens a stream only after every older stream ended.                                         |
| deadlock               | TLC check | Work never stops while handlers still have requests.                                          |
| `BorrowerRecovers`     | liveness  | After the last fault, A's shared node gets a working stream to D and keeps it.                |
| `AtMostOnceExecution`  | contract  | A's handler runs each of D's calls at most once.                                              |
| `PerStreamFIFO`        | contract  | A starts the calls of one stream in the order that D admitted them.                           |
| `CrossStreamFIFO`      | limit     | A starts all of D's calls in the order that D admitted them.                                  |
| `NoCrossStreamOverlap` | limit     | A's handlers run D's calls one at a time across streams.                                      |

`PerStreamFIFO` checks the user guide's statement "Requests carried by one stream start in the order that the sender sent them".
A limit is a property that the contract excludes; its configuration gives a trace that breaks it.
The FIFO properties need the history variables in `StreamFIFO.tla`.

## Running the Checks

Run the commands from the repository root; [spec/README.md](../README.md) explains the tools.
`just spec-list` lists the configurations.
On an error, `just tlc` prints one line per step, with only the variables that changed.
The times below are from one laptop and vary with the machine.

### Current Code

`Current.cfg`: all invariants, deadlock, and `BorrowerRecovers`.
2 calls from D, 1 from A, capacity 1, 2 faults; callers may cancel.
Passes: 882,739 distinct states, 51 s.

```bash
just tlc Current
```

`CurrentLate.cfg`: as `Current.cfg`, with `LateFrames`.
Passes: 1.4 M distinct states, 78 s.

```bash
just tlc CurrentLate
```

`ContractAtMostOnce.cfg`: `AtMostOnceExecution` with `LateFrames`, 2 calls, 2 faults.
Passes: 13,002 distinct states, 1 s.

```bash
just tlc ContractAtMostOnce
```

`ContractFIFO.cfg`: `PerStreamFIFO` with `LateFrames`, 3 calls, queue capacity 2, 2 faults.
Passes: 343,444 distinct states, 4 s.

```bash
just tlc ContractFIFO
```

### Ordering Boundary

`LimitCrossStreamFIFO.cfg`: `CrossStreamFIFO`, 2 calls, queue capacity 2, 1 fault.
Fails, 1 s.

```bash
just tlc LimitCrossStreamFIFO
```

`LimitCrossStreamOverlap.cfg`: `NoCrossStreamOverlap`, with the same bounds.
Fails, 1 s.

```bash
just tlc LimitCrossStreamOverlap
```

### Past Bugs

`BugLazyReconnect.cfg`: `BorrowerRecovers` with `LazyReconnect`.
Fails, 1 s.

```bash
just tlc BugLazyReconnect
```

`BugAttachClosesPrior.cfg`: `SlotTracksLive` with `AttachClosesPrior`.
Fails, 1 s.

```bash
just tlc BugAttachClosesPrior
```

`BugBlockingReplies.cfg`: deadlock with `BlockingReplies`.
5 calls in each direction, no faults, call symmetry.
Deadlocks, 13 min.

```bash
just tlc BugBlockingReplies
```

`BugRequeue.cfg`: `AtMostOnceExecution` with `Requeue`, 1 call, 1 fault.
Fails, 1 s.

```bash
just tlc BugRequeue
```

`BugRequeueFIFO.cfg`: `PerStreamFIFO` with `Requeue`, 2 calls, 1 fault.
Fails, 1 s.

```bash
just tlc BugRequeueFIFO
```

## The Model Finds the Four Past Bugs

Each bug switch puts back the core of a past bug, in terms of today's code.
TLC finds each one, so the model is detailed enough to catch them.

**Lazy reconnect** (dual-mode startup failures and the dedup wedge, 2026-07).
The stream breaks while D has no requests.
D waits for a request before it reopens, so A's shared node has no stream, forever.
Today's code reconnects eagerly for every symmetric outbound channel.

**Attach closes prior** (duplicate inbound stream drops peer, 2026-07).
D's stream 1 breaks, and D opens stream 2.
A attaches stream 2 first, then the late start of stream 1.
Attaching stream 1 closes the channel of stream 2, which is healthy.
When stream 1 ends, its detach clears the slot.
Stream 2 is alive, but A's shared node has no channel.
Today's `liveChannels` fails over to the latest live channel.

**Blocking replies** (stream-dedup teardown deadlock, 2026-07).
If a handler's reply waits for send queue space, both peers can deadlock:

![Six-way wait cycle between D's and A's goroutines](figures/wait-cycle.svg)

D's goroutines are blue and A's are green.
With capacity 1, the cycle needs 5 calls in each direction.
Today's code drops a reply when the queue is full (`DroppedReplies`), so the cycle cannot form.

**Requeue** (calls ran twice, fixed for issue 468).
When a stream broke, D sent each pending call again on the next stream.
If A had already run the handler and only the reply was lost, A ran it a second time:

![c1 runs twice: once on stream 1 and again on stream 2](figures/at-least-once.svg)

This is the trace of `BugRequeue.cfg`; a lost reply has the same effect.
The requeued call also went behind newer calls on the next stream, which broke FIFO order within that stream (`BugRequeueFIFO.cfg`).
Today's code fails each pending call with `ErrStreamDown`, so it runs each call at most once.
The caller of such a call cannot tell whether A ran it.

The per-send cancel watcher bug is not a switch.
Today's code has no per-send watcher.
Its effect was a stream that was never reopened, which the lazy-reconnect switch covers.

## A New Stream Is an Ordering Boundary

A starts the calls of one stream in the order that D admitted them (`ContractFIFO.cfg`).
This order does not extend across D's streams, because A gives each inbound stream its own dispatcher.
After a break, A can start a call that arrived on stream 1 after it has started a later call from stream 2:

![c2 runs before c1, because each channel has its own dispatcher](figures/dispatcher-race.svg)

This is the trace of `LimitCrossStreamFIFO.cfg`.
`c1` waits in channel 1's dispatcher while `c2` runs on channel 2.
D has already failed `c1` with `ErrStreamDown`, so D's caller knows that `c1` may still run.

The handlers of the two streams can also overlap: `c1` still runs on channel 1 when `c2` starts on channel 2 (`LimitCrossStreamOverlap.cfg`).
With `LateFrames`, a call can even arrive on stream 1 after A has started calls from stream 2.

An application that needs an order across streams must carry its own sequence numbers or epochs in its requests.

## Limits

* The bounds are small: few calls, capacity 1, and at most 2 faults.
  TLC proves the properties only for these bounds.
* With `Requeue`, `EndD` requeues pending calls in one fixed order; the code used map order.
* The model is written from the code by hand.
  Nothing checks yet that the code behaves as the model says.

## Next Steps

1. Add refusals with backoff, and GOAWAY with `Drain`.
   These are the stream-layer paths with the most recent changes.
2. Add one-way calls, which have no pending entry and no `ErrStreamDown`.
3. Add trace validation: log the model's actions from `gorumstest` runs, and check each log against the spec with TLC.

## Files

| File             | Contents                                               |
| ---------------- | ------------------------------------------------------ |
| `Stream.tla`     | The model.                                             |
| `StreamFIFO.tla` | The model with history variables, for the FIFO checks. |
| `*.cfg`          | TLC configurations, one per check above.               |
| `figures/`       | The figures in this README.                            |
