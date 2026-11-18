# TLA+ Model of the Stream Layer

This model covers the Gorums stream layer for one peer pair under stream deduplication.
TLC checks it for small bounds.

* The current code passes every invariant, the deadlock check, the liveness property `BorrowerRecovers`, and three contracts: each call runs at most once, one sender's calls run in FIFO order across reconnects, and one at a time.
* Its one known limit: after a GOAWAY that arrives within one round trip of opening a stream, that stream's calls fail.
* The model finds the five past stream-layer bugs when a switch puts each one back.
  One of them is the FIFO race across reconnects that issue 468 fixed.
* Two simpler designs fail: a gate that waits until older channels are idle loses FIFO order when a break leaves frames in transit.
  Plain supersede, where a newer stream from a sender replaces that sender's older stream at the server, loses calls after a GOAWAY.

## What the Model Covers

D is the lower-ID peer and the only dialer.
A is the higher-ID peer.
A's outbound node for D is born shared, so A's calls to D use D's stream.
D sends on one live stream at a time.
After a GOAWAY, D drains the old stream: it sends nothing more on it and half-closes it, but keeps receiving its replies.
New calls go on a new stream.
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
| `GoAwayD`, `DrainEndD`                   | `OutboundChannel.watchGoAway`, `session.startDrain`, `receiveFailed`    |
| `SealS`, `FinishS`                       | `InboundChannel.Serve` after the dialer half-closed the stream          |
| `IssueB`                                 | `Transport.Enqueue` on the shared transport                             |
| `Stale`, `Kept`, `SupersedeGate`         | `Server.NodeStream`, `sender`, `dispatcher.hold`, `waitRetired`         |
| `dmeta`, `dacc`                          | `OutboundChannel.streamMetadata`, the `gorums-accepted` header          |

Not modeled yet: refusals and their backoff, streaming and one-way calls, `ResourceExhausted`, `LocalChannel`, and application `Close`.
`MaxGoAways` bounds the GOAWAYs, as `MaxFaults` bounds the breaks.

### Switches

Each configuration sets every switch.
The bug switches off and `FIFOGate = "drain"` model the current code.

| Switch                   | Kind    | Effect                                                                                                                                                    |
| ------------------------ | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `LazyReconnect`          | bug     | D reopens a lost stream only when it has a request.                                                                                                       |
| `AttachClosesPrior`      | bug     | Attaching a stream closes the prior channel.                                                                                                              |
| `BlockingReplies`        | bug     | A handler's reply waits for send queue space.                                                                                                             |
| `Requeue`                | bug     | A lost stream requeues its pending calls at the tail of the send queue.                                                                                   |
| `FIFOGate = "none"`      | design  | No gate, as before issue 468. Each inbound stream has its own dispatcher.                                                                                 |
| `FIFOGate = "idle"`      | design  | A starts a handler on stream `s` only while each older channel is detached or idle.                                                                       |
| `FIFOGate = "supersede"` | design  | A refuses a stream older than one it has seen. A newer stream closes the older channels, and its handlers wait for their running handlers.                |
| `FIFOGate = "drain"`     | current | As supersede, but D lists its accepted draining streams in a new stream's metadata. A keeps those, and the new stream's handlers wait until each one has finished. |
| `LateFrames`             | network | A break loses only a suffix of the frames in flight. The receiver still gets the rest.                                                                    |
| `LostStarts`             | network | The start of a broken stream may never reach A.                                                                                                           |

To supersede a stream means that a newer stream from D replaces it at A.
A closes the channel of the older stream, and refuses the older stream if its start arrives late.
Every gate needs D's stream number at A; the code sends it in the stream metadata.
`LateFrames` models data that is still in transit or in buffers when the sender gives up, as after a keepalive timeout.

## Properties

| Property              | Kind      | Meaning                                                                                                         |
| --------------------- | --------- | --------------------------------------------------------------------------------------------------------------- |
| `AtMostOnceDelivery`  | invariant | The runtime delivers at most one response to each call.                                                         |
| `NoDuplicateCall`     | invariant | A call is in at most one send queue or pending table.                                                           |
| `SlotTracksLive`      | invariant | While A serves a stream that D still sends on and that is not broken, the shared node's slot holds an attached channel. |
| `OlderStreamsEnded`   | invariant | D opens a stream only after every older stream ended or started draining.                                       |
| `DrainSafe`           | invariant | Without a break, a call fails with `ErrStreamDown` only if A dropped its reply because A's send queue was full. |
| `ListedDrainSafe`     | invariant | As `DrainSafe`, but a call may also fail if it was pending on a draining stream that D did not list.             |
| deadlock              | TLC check | Work never stops while handlers still have requests.                                                            |
| `BorrowerRecovers`    | liveness  | After the last fault, A's shared node gets a working stream to D and keeps it.                                  |
| `AtMostOnceExecution` | contract  | A's handler runs each of D's calls at most once.                                                                |
| `PerSenderFIFO`       | contract  | A starts D's calls in the order that D admitted them.                                                           |
| `OneAtATime`          | contract  | A's handlers run D's calls one at a time.                                                                       |

`PerSenderFIFO` checks the user guide's statement "Messages from the same sender are executed in FIFO order".
It needs the history variables in `StreamFIFO.tla`.

## Running the Checks

Run the commands from the repository root; [spec/README.md](../README.md) explains the tools.
`just spec-list` lists the configurations.
On an error, `just tlc` prints one line per step, with only the variables that changed.
The times below are from one laptop and vary with the machine.

### Current Code

`Current.cfg`: all invariants, both contracts, `OneAtATime`, deadlock, and `BorrowerRecovers`.
2 calls from D, 1 from A, capacity 1, 2 faults; callers may cancel.
Passes: 1,709,865 distinct states, 133 s.

```bash
just tlc Current
```

`CurrentLate.cfg`: as `Current.cfg`, with `LateFrames`.
Passes: 2,253,948 distinct states, 203 s.

```bash
just tlc CurrentLate
```

`CurrentDrain.cfg`: as `Current.cfg`, with `ListedDrainSafe`, one GOAWAY, one break, and `LateFrames`, without cancel.
Passes: 2,352,176 distinct states, 203 s.

```bash
just tlc CurrentDrain
```

`CurrentLostStart.cfg`: as `CurrentDrain.cfg`, with `LostStarts` and without A's calls or `LateFrames`.
Passes: 156,196 distinct states, 11 s.

```bash
just tlc CurrentLostStart
```

`ContractAtMostOnce.cfg`: `AtMostOnceExecution` with `LateFrames`, 2 calls, 2 faults.
Passes: 30,040 distinct states, 2 s.

```bash
just tlc ContractAtMostOnce
```

`LimitUnaccepted.cfg`: `DrainSafe` with one GOAWAY; the known limit.
Fails, 1 s.

```bash
just tlc LimitUnaccepted
```

### Past Bugs

`BugLazyReconnect.cfg`: `BorrowerRecovers` with `LazyReconnect`.
Fails, 1 s.

```bash
just tlc BugLazyReconnect
```

`BugAttachClosesPrior.cfg`: `SlotTracksLive` with `AttachClosesPrior`, without a gate.
Fails, 1 s.

```bash
just tlc BugAttachClosesPrior
```

`BugBlockingReplies.cfg`: deadlock with `BlockingReplies`.
5 calls in each direction, no faults, call symmetry.
Deadlocks, 7 min.

```bash
just tlc BugBlockingReplies
```

`BugRequeue.cfg`: `AtMostOnceExecution` with `Requeue`, 1 call, 1 fault.
Fails, 1 s.

```bash
just tlc BugRequeue
```

`BugRequeueFIFO.cfg`: `PerSenderFIFO` with `Requeue` and `LateFrames`.
Fails, 1 s.

```bash
just tlc BugRequeueFIFO
```

`BugNoGateFIFO.cfg`: `PerSenderFIFO` without a gate, 2 calls, queue capacity 2, 1 fault.
Fails, 1 s.

```bash
just tlc BugNoGateFIFO
```

`BugNoGateDrainSlot.cfg`: `SlotTracksLive` without a gate, with one GOAWAY.
Fails, 1 s.

```bash
just tlc BugNoGateDrainSlot
```

### Rejected Designs

`AltIdle.cfg`: `PerSenderFIFO` with the idle gate; a break loses all frames in flight.
Passes: 1,274 distinct states, 1 s.

```bash
just tlc AltIdle
```

`AltIdleLate.cfg`: as `AltIdle.cfg`, with `LateFrames`.
Fails, 1 s.

```bash
just tlc AltIdleLate
```

`AltSupersedeLate.cfg`: `PerSenderFIFO` with plain supersede and `LateFrames`, 3 calls, 2 faults.
Passes: 126,638 distinct states, 6 s.

```bash
just tlc AltSupersedeLate
```

`AltSupersedeOneAtATime.cfg`: `OneAtATime` with plain supersede and `LateFrames`.
Passes: 1,311 distinct states, 1 s.

```bash
just tlc AltSupersedeOneAtATime
```

`AltSupersedeDrain.cfg`: `DrainSafe` with plain supersede and one GOAWAY.
Fails, 1 s.

```bash
just tlc AltSupersedeDrain
```

## The Model Finds the Past Bugs

Each bug switch puts back the core of a past bug, in terms of today's code.
TLC finds each one, so the model is detailed enough to catch them.

**Lazy reconnect** (dual-mode startup failures and the dedup wedge, 2026-07).
The stream breaks while D has no requests.
D waits for a request before it reopens, so A's shared node has no stream, forever.
Today's code reconnects eagerly for every symmetric outbound channel.

**Attach closes prior** (duplicate inbound stream drops peer, 2026-07).
This check runs without a gate, since the current gate refuses the late stream that triggers the bug.
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

**No FIFO gate** (calls ran out of order across a reconnect, fixed for issue 468).
The user guide says that messages from the same sender run in FIFO order at all servers.
That held per stream, but not per sender across a stream break, because A gives each inbound stream its own dispatcher.
After a break, A could still run requests that arrived on stream 1 while it already ran requests from stream 2:

![c2 runs before c1, because each channel has its own dispatcher](figures/dispatcher-race.svg)

This is the trace of `BugNoGateFIFO.cfg`.
`c1` waits in channel 1's dispatcher while `c2` runs on channel 2.
Without a gate, a draining stream that A attaches late also becomes the shared node's slot, so A's calls to D fail until it detaches (`BugNoGateDrainSlot.cfg`).

## How the Current Code Keeps FIFO Order

FIFO order needs a gate at A.

**The idle gate is not enough.**
The idle gate lets A start a handler on stream 2 only while channel 1 is detached or has no queued or running request.
This fixes the race above, and `AltIdle.cfg` passes.
It fails when a break leaves a frame in transit (`AltIdleLate.cfg`):

![A late c1 runs after c2, although the idle gate is on](figures/late-frame-idle-gate.svg)

The slanted arrow is a frame in transit.
A cannot know if more data will come on stream 1 until stream 1 ends at A.
A gate that waits for stream 1 to end is correct, but it stalls stream 2 until A detects the break, which can take a keepalive timeout.

**Supersede holds.**
With supersede, A refuses a stream older than one it has seen.
When a newer stream attaches, A closes the older channel and drops its queued and late requests.
D has already failed those calls with `ErrStreamDown`, so at-most-once permits the drop.
`AltSupersedeLate.cfg` passes.

![Supersede closes channel 1, so the late c1 is dropped](figures/late-frame-supersede.svg)

Supersede is not the old `AttachClosesPrior` bug.
The bug ordered streams by attach order, so a late start of an old stream closed the new one.
Supersede orders streams by D's stream number, and it refuses the late old stream.

Supersede closes the older channel, but a handler that already runs on it continues.
So the new channel's handlers wait until that handler returns or releases, and `OneAtATime` holds (`AltSupersedeOneAtATime.cfg`).

## GOAWAY Draining

After a GOAWAY, D keeps the old stream for the replies of the calls it already sent there, and sends new calls on a new stream.
A stream that drains this way has no late frames: D half-closed it, so every frame on it precedes its end.

**Plain supersede loses calls.**
A treats the old stream like a lost one: it closes it, or refuses it if its start arrives after the new stream's.
The calls that D sent on it then fail with `ErrStreamDown`, although nothing broke (`AltSupersedeDrain.cfg`).

**The drain gate holds; the code implements it.**
A sends an acceptance on each stream after it attaches the channel, as the `gorums-accepted` header does.
D lists in the new stream's metadata the draining streams whose acceptance it has received.
A keeps those streams, closes only the others, and starts the new stream's handlers only after each kept stream has run all its requests.
A kept stream that A attaches late takes the slot only if the slot has no attached channel.
`CurrentDrain.cfg` passes every property, including `ListedDrainSafe`, `PerSenderFIFO`, `OneAtATime`, and `BorrowerRecovers`.

**Why D lists only accepted streams.**
A draining stream can also break; a connection that breaks leaves Ready, which D handles as a GOAWAY.
The start of such a stream may never reach A.
If D listed it, A could not tell a late start from a lost one, and the new stream's handlers would wait forever.
A listed stream is one that A has attached, so A always sees it finish.
`CurrentLostStart.cfg` passes with `LostStarts`.

**The cost.**
If a GOAWAY arrives before the acceptance, D does not list the stream, and A closes it.
The calls that D sent on it fail with `ErrStreamDown`, although nothing broke (`LimitUnaccepted.cfg`).
This window is one round trip after D opens a stream.

## Limits

* The bounds are small: few calls, capacity 1, and at most 2 faults.
  TLC proves the properties only for these bounds.
* With `Requeue`, `EndD` requeues pending calls in one fixed order; the code used map order.
* The model is written from the code by hand.
  Nothing checks yet that the code behaves as the model says.

## Next Steps

1. Add refusals with backoff, and the application's `Drain`.
2. Add trace validation: log the model's actions from `gorumstest` runs, and check each log against the spec with TLC.

## Files

| File             | Contents                                               |
| ---------------- | ------------------------------------------------------ |
| `Stream.tla`     | The model.                                             |
| `StreamFIFO.tla` | The model with history variables, for `PerSenderFIFO`. |
| `*.cfg`          | TLC configurations, one per check above.               |
| `figures/`       | The figures in this README.                            |
