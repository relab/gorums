# TLA+ Model of the Stream Layer

This model covers the Gorums stream layer for one peer pair under stream deduplication.
TLC checks it for small bounds.

* The current code passes every invariant, the deadlock check, and the liveness property `BorrowerRecovers`.
* The model finds the four past stream-layer bugs when a switch puts each one back.
* The current code runs each call at most once.
  It breaks one documented contract: calls from one sender can run out of FIFO order across a reconnect.
* A proposed design, named supersede, fixes this: a newer stream from a sender replaces that sender's older stream at the server.
  The code does not implement it yet.
* A simpler gate, "wait until older channels are idle", fails when a break leaves frames in transit.

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
| `FIFOGate = "idle"`      | design  | A starts a handler on stream `s` only while each older channel is detached or idle.     |
| `FIFOGate = "supersede"` | design  | A refuses a stream older than one it has seen. A newer stream closes the older channel. |
| `LateFrames`             | network | A break loses only a suffix of the frames in flight. The receiver still gets the rest.  |

To supersede a stream means that a newer stream from D replaces it at A.
A closes the channel of the older stream, and refuses the older stream if its start arrives late.
Both gates need D's stream number at A, for example in the stream metadata.
`LateFrames` models data that is still in transit or in buffers when the sender gives up, as after a keepalive timeout.

## Properties

| Property              | Kind      | Meaning                                                                                       |
| --------------------- | --------- | --------------------------------------------------------------------------------------------- |
| `AtMostOnceDelivery`  | invariant | The runtime delivers at most one response to each call.                                       |
| `NoDuplicateCall`     | invariant | A call is in at most one send queue or pending table.                                         |
| `SlotTracksLive`      | invariant | While A serves a stream that is not broken, the shared node's slot holds an attached channel. |
| `OlderStreamsEnded`   | invariant | D opens a stream only after every older stream ended.                                         |
| deadlock              | TLC check | Work never stops while handlers still have requests.                                          |
| `BorrowerRecovers`    | liveness  | After the last fault, A's shared node gets a working stream to D and keeps it.                |
| `AtMostOnceExecution` | contract  | A's handler runs each of D's calls at most once.                                              |
| `PerSenderFIFO`       | contract  | A starts D's calls in the order that D admitted them.                                         |
| `OneAtATime`          | contract  | A's handlers run D's calls one at a time.                                                     |

`PerSenderFIFO` checks the user guide's statement "Messages from the same sender are executed in FIFO order".
It needs the history variables in `StreamFIFO.tla`.

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

`ContractFIFO.cfg`: `PerSenderFIFO`, 2 calls, queue capacity 2, 1 fault.
Fails, 1 s.

```bash
just tlc ContractFIFO
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

`BugRequeueFIFO.cfg`: `PerSenderFIFO` with `Requeue`, supersede, and `LateFrames`.
Fails, 1 s.

```bash
just tlc BugRequeueFIFO
```

### Proposed Design

`FixFIFOIdle.cfg`: `PerSenderFIFO` with the idle gate; a break loses all frames in flight.
Passes: 1,274 distinct states, 1 s.

```bash
just tlc FixFIFOIdle
```

`FixFIFOIdleLate.cfg`: as `FixFIFOIdle.cfg`, with `LateFrames`.
Fails, 1 s.

```bash
just tlc FixFIFOIdleLate
```

`FixFIFOSupersedeLate.cfg`: `PerSenderFIFO` with supersede and `LateFrames`, 3 calls, 2 faults.
Passes: 107,910 distinct states, 2 s.

```bash
just tlc FixFIFOSupersedeLate
```

`FixOneAtATime.cfg`: `OneAtATime` with supersede and `LateFrames`.
Fails, 1 s.

```bash
just tlc FixOneAtATime
```

`Proposed.cfg`: everything in `Current.cfg`, plus both contracts, with supersede and `LateFrames`.
Passes: 658,162 distinct states, 45 s.

```bash
just tlc Proposed
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
The requeued call also went behind newer calls, which broke FIFO order (`BugRequeueFIFO.cfg`).
Today's code fails each pending call with `ErrStreamDown`, so it runs each call at most once.
The caller of such a call cannot tell whether A ran it.

The per-send cancel watcher bug is not a switch.
Today's code has no per-send watcher.
Its effect was a stream that was never reopened, which the lazy-reconnect switch covers.

## FIFO Order per Sender Does Not Hold Across a Reconnect

The user guide says that messages from the same sender run in FIFO order at all servers.
That is true per stream.
It is not true per sender across a stream break, because A gives each inbound stream its own dispatcher.
After a break, A can still run requests that arrived on stream 1 while it already runs requests from stream 2:

![c2 runs before c1, because each channel has its own dispatcher](figures/dispatcher-race.svg)

This is the trace of `ContractFIFO.cfg`.
`c1` waits in channel 1's dispatcher while `c2` runs on channel 2.

## The Proposed Design

FIFO order needs a gate at A.

**The idle gate is not enough.**
The idle gate lets A start a handler on stream 2 only while channel 1 is detached or has no queued or running request.
This fixes the race above, and `FixFIFOIdle.cfg` passes.
It fails when a break leaves a frame in transit (`FixFIFOIdleLate.cfg`):

![A late c1 runs after c2, although the idle gate is on](figures/late-frame-idle-gate.svg)

The slanted arrow is a frame in transit.
A cannot know if more data will come on stream 1 until stream 1 ends at A.
A gate that waits for stream 1 to end is correct, but it stalls stream 2 until A detects the break, which can take a keepalive timeout.

**Supersede holds.**
With supersede, A refuses a stream older than one it has seen.
When a newer stream attaches, A closes the older channel and drops its queued and late requests.
D has already failed those calls with `ErrStreamDown`, so at-most-once permits the drop.
`FixFIFOSupersedeLate.cfg` and `Proposed.cfg` pass.

![Supersede closes channel 1, so the late c1 is dropped](figures/late-frame-supersede.svg)

Supersede is not the old `AttachClosesPrior` bug.
The bug ordered streams by attach order, so a late start of an old stream closed the new one.
Supersede orders streams by D's stream number, and it refuses the late old stream.

Supersede closes the older channel, but a handler that already runs on it continues.
So the handlers of `c1` and `c2` can overlap, although `c1` starts first; `FixOneAtATime.cfg` shows this.
If FIFO must also mean "one at a time", the new channel's dispatcher must wait for that handler to return.

## Limits

* The bounds are small: few calls, capacity 1, and at most 2 faults.
  TLC proves the properties only for these bounds.
* With `Requeue`, `EndD` requeues pending calls in one fixed order; the code used map order.
* The model is written from the code by hand.
  Nothing checks yet that the code behaves as the model says.

## Next Steps

1. Implement supersede, with a stream number in the stream metadata.
2. Add refusals with backoff, and GOAWAY with `Drain`.
   These are the stream-layer paths with the most recent changes.
3. Add trace validation: log the model's actions from `gorumstest` runs, and check each log against the spec with TLC.

## Files

| File             | Contents                                               |
| ---------------- | ------------------------------------------------------ |
| `Stream.tla`     | The model.                                             |
| `StreamFIFO.tla` | The model with history variables, for `PerSenderFIFO`. |
| `*.cfg`          | TLC configurations, one per check above.               |
| `figures/`       | The figures in this README.                            |
