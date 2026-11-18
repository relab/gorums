------------------------------- MODULE Stream -------------------------------
(***************************************************************************)
(* A model of the Gorums stream layer for one peer pair under stream       *)
(* deduplication.                                                          *)
(*                                                                         *)
(* D is the lower-ID peer. It is the only dialer. Its OutboundChannel      *)
(* sends D's calls on one live stream at a time. After a GOAWAY, an older  *)
(* stream drains while the live stream carries new calls.                  *)
(*                                                                         *)
(* A is the higher-ID peer. Its server accepts D's streams. Each accepted  *)
(* stream gets an InboundChannel, which is attached to A's known-peer node *)
(* for D. A's outbound node for D is born shared: it borrows that node's   *)
(* channel slot, so A's calls to D travel on D's stream as                 *)
(* server-initiated requests.                                              *)
(*                                                                         *)
(* Code map:                                                               *)
(*   dq, dhand, dlive, OpenD, TakeD, SendD, EndD, GoAwayD, DrainEndD       *)
(*        internal/stream/outbound.go (run, watchGoAway) and session.go    *)
(*   dpend, apend    session.pending (pendingCalls)                        *)
(*   ddisp, adisp    endpoint.requests (dispatcher, RecvBufferSize)        *)
(*   ReplyD, ReplyS  endpoint.reply                                        *)
(*   AcceptS, ServerEndS, FinishS, live, slot                              *)
(*        internal/conn/node.go (attachStream, liveChannels)               *)
(*   Stale, Kept, SupersedeGate, dmeta, dacc                               *)
(*        internal/stream/server.go (Server.NodeStream, sender) and        *)
(*        outbound.go (streamMetadata)                                     *)
(*   SealS           InboundChannel.Serve after the dialer half-closed     *)
(*   IssueB          Transport.Enqueue on the shared transport             *)
(*                                                                         *)
(* Not modeled: refusals and their backoff, streaming calls, one-way       *)
(* calls, ResourceExhausted, LocalChannel, and channel Close by the        *)
(* application.                                                            *)
(***************************************************************************)
EXTENDS Naturals, Sequences, FiniteSets, TLC

CONSTANTS
    DCalls,             \* calls that D sends to A
    BCalls,             \* calls that A's shared node sends to D
    QCap,               \* send queue capacity, on both sides
    WireCap,            \* frames in flight per stream direction (flow control)
    RecvBuf,            \* dispatcher buffer (RecvBufferSize)
    MaxFaults,          \* number of stream breaks the network may inject
    MaxGoAways,         \* number of GOAWAYs that A's server may send
    AllowCancel,        \* callers may give up (context deadline)
    \* Bug switches. All FALSE models the current code.
    LazyReconnect,      \* D reopens a lost stream only when it has a request
    AttachClosesPrior,  \* attach closes the prior channel; detach clears the slot only if it holds its own channel
    BlockingReplies,    \* a handler's reply waits for send queue space
    Requeue,            \* a lost stream requeues its pending calls at the tail of the send queue
    \* Design switch. "drain" models the current code, "none" the code
    \* before issue 468, and "idle" and "supersede" rejected designs.
    FIFOGate,           \* "none", "idle", "supersede" (a newer stream replaces the older one at A), or "drain"; see AcceptS and HandleS
    \* Network switches.
    LateFrames,         \* a break loses only a suffix of the frames in flight
    LostStarts          \* the start of a broken stream may never reach A

ASSUME QCap >= 1 /\ WireCap >= 1 /\ RecvBuf >= 1
ASSUME MaxFaults \in Nat /\ MaxGoAways \in Nat
ASSUME FIFOGate \in {"none", "idle", "supersede", "drain"}
ASSUME DCalls \cap BCalls = {}

\* A stream stops being D's live stream only when it breaks or drains.
Streams == 1..(MaxFaults + MaxGoAways + 1)

\* Both supersede variants: D numbers its streams, and A knows the numbers.
Supersede == FIFOGate \in {"supersede", "drain"}

VARIABLES
    \* D, the dialer.
    dq,        \* send queue: a sequence of items
    dhand,     \* <<>> or <<[item, added]>>: an item taken from dq, not yet sent
    dlive,     \* the stream of the live session, or 0
    dstarted,  \* the channel has made its first connect
    ddisp,     \* requests from A waiting for D's handler
    drun,      \* <<>> or <<b>>: the request D's handler is running
    dpend,     \* dpend[s]: D's calls pending on stream s
    cst,       \* cst[s]: D's view of stream s: "none", "open", "draining", or "ended"
    dfin,      \* dfin[s]: D half-closed s after a GOAWAY
    dmeta,     \* dmeta[s]: the streams that D listed as draining when it opened s
    dacc,      \* dacc[s]: D received A's acceptance of s (FIFOGate = "drain")
    \* The network.
    c2s,       \* c2s[s]: frames in flight from D to A on stream s
    s2c,       \* s2c[s]: frames in flight from A to D on stream s
    broken,    \* broken[s]: the network broke stream s
    faults,    \* number of breaks so far
    goaways,   \* number of GOAWAYs so far
    \* A, the acceptor.
    sst,       \* sst[s]: A's inbound channel for s: "none", "attached",
               \*   "sealed" (D half-closed s), "closed", "detached", or
               \*   "lost" (the stream's start never reaches A)
    aq,        \* aq[s]: the inbound channel's send queue
    ahand,     \* ahand[s]: as dhand
    adisp,     \* adisp[s]: requests from D waiting for A's handler
    arun,      \* arun[s]: as drun
    apend,     \* apend[s]: A's calls pending on stream s
    live,      \* Node.liveChannels for D, in attach order
    slot,      \* the stream behind the node's channelRef, or 0 for nil
    sfin,      \* sfin[s]: A ended s without a break (after D's half-close, or a refusal)
    \* Calls.
    dst,       \* dst[c]: "idle", "active", "ok", "err", "down", or "gone"
    ddel,      \* ddel[c]: responses the runtime delivered to c's caller
    handled,   \* handled[c]: times A's handler ran c
    qdrop,     \* D's calls whose replies A dropped because its send queue was full
    unlisted,  \* D's calls pending on a draining stream that D did not list
    bst,       \* bst[b]: as dst
    bdel       \* bdel[b]: as ddel

dvars == <<dq, dhand, dlive, dstarted, ddisp, drun, dpend, cst, dfin, dmeta, dacc>>
wvars == <<c2s, s2c, broken, faults, goaways>>
avars == <<sst, aq, ahand, adisp, arun, apend, live, slot, sfin>>
cvars == <<dst, ddel, handled, qdrop, unlisted, bst, bdel>>
vars  == <<dvars, wvars, avars, cvars>>

-----------------------------------------------------------------------------
(* Helpers                                                                 *)

Item(kind, x) == [k |-> kind, id |-> x]

\* The call IDs among the items of q.
CallsIn(q) == {q[i].id : i \in {j \in DOMAIN q : q[j].k = "call"}}

\* Marks the active calls in S with outcome o.
Mark(f, S, o) == [x \in DOMAIN f |-> IF x \in S /\ f[x] = "active" THEN o ELSE f[x]]

\* Counts one delivery for each call in S.
Count(g, S) == [x \in DOMAIN g |-> IF x \in S THEN g[x] + 1 ELSE g[x]]

\* Some fixed order of the elements of S. The code uses map order.
SetToSeq(S) == CHOOSE f \in [1..Cardinality(S) -> S] :
                    \A i, j \in DOMAIN f : i # j => f[i] # f[j]

Min(a, b) == IF a < b THEN a ELSE b

-----------------------------------------------------------------------------
(* Callers                                                                 *)

\* D's call c: Transport.Enqueue on D's owned outbound channel.
IssueD(c) ==
    /\ dst[c] = "idle"
    /\ IF Len(dq) < QCap
         THEN /\ dq' = Append(dq, Item("call", c))
              /\ dst' = [dst EXCEPT ![c] = "active"]
         ELSE /\ dst' = [dst EXCEPT ![c] = "err"]      \* ErrSendQueueFull
              /\ UNCHANGED dq
    /\ UNCHANGED <<dhand, dlive, dstarted, ddisp, drun, dpend, cst, dfin, dmeta, dacc>>
    /\ UNCHANGED <<wvars, avars, ddel, handled, qdrop, unlisted, bst, bdel>>

\* A's call b: Transport.Enqueue on the shared transport, which loads the slot.
IssueB(b) ==
    /\ bst[b] = "idle"
    /\ IF slot # 0 /\ sst[slot] = "attached" /\ Len(aq[slot]) < QCap
         THEN /\ aq' = [aq EXCEPT ![slot] = Append(@, Item("call", b))]
              /\ bst' = [bst EXCEPT ![b] = "active"]
         ELSE /\ bst' = [bst EXCEPT ![b] = "err"]      \* ErrStreamDown or ErrSendQueueFull
              /\ UNCHANGED aq
    /\ UNCHANGED <<dvars, wvars, sst, ahand, adisp, arun, apend, live, slot, sfin>>
    /\ UNCHANGED <<dst, ddel, handled, qdrop, unlisted, bdel>>

\* A caller's context ends. The runtime does not learn of it until it next
\* touches the call.
CancelD(c) ==
    /\ AllowCancel /\ dst[c] = "active"
    /\ dst' = [dst EXCEPT ![c] = "gone"]
    /\ UNCHANGED <<dvars, wvars, avars, ddel, handled, qdrop, unlisted, bst, bdel>>

CancelB(b) ==
    /\ AllowCancel /\ bst[b] = "active"
    /\ bst' = [bst EXCEPT ![b] = "gone"]
    /\ UNCHANGED <<dvars, wvars, avars, dst, ddel, handled, qdrop, unlisted, bdel>>

-----------------------------------------------------------------------------
(* D: OutboundChannel.run, session.sendLoop, session.receive               *)

\* run opens a stream at startup. After a lost or draining stream it opens the
\* next one at once if it holds a request, and otherwise under eager
\* reconnect, or when a request arrives. With FIFOGate = "drain", the stream
\* metadata lists the draining streams that A has accepted. The calls pending
\* on an unlisted draining stream are recorded in unlisted.
OpenD ==
    /\ dlive = 0
    /\ \/ ~dstarted
       \/ ~LazyReconnect
       \/ dq # <<>>
       \/ dhand # <<>>
    /\ \E s \in Streams :
         /\ cst[s] = "none"
         /\ \A t \in Streams : t < s => cst[t] # "none"
         /\ cst' = [cst EXCEPT ![s] = "open"]
         /\ dlive' = s
         /\ dmeta' = [dmeta EXCEPT ![s] = {t \in Streams : cst[t] = "draining" /\ dacc[t]}]
    /\ unlisted' = IF FIFOGate = "drain"
                     THEN unlisted \cup UNION {dpend[t] : t \in {u \in Streams :
                                                 cst[u] = "draining" /\ ~dacc[u]}}
                     ELSE unlisted
    /\ dstarted' = TRUE
    /\ UNCHANGED <<dq, dhand, ddisp, drun, dpend, dfin, dacc, wvars, avars>>
    /\ UNCHANGED <<dst, ddel, handled, qdrop, bst, bdel>>

\* sendLoop takes a request. A call whose caller is gone fails at once.
\* Otherwise a two-way call becomes pending before it is sent.
TakeD ==
    /\ dlive # 0 /\ dhand = <<>> /\ dq # <<>>
    /\ LET it == Head(dq) IN
         /\ dq' = Tail(dq)
         /\ IF it.k = "call" /\ dst[it.id] = "gone"
              THEN /\ ddel' = Count(ddel, {it.id})
                   /\ UNCHANGED <<dhand, dpend>>
              ELSE /\ dhand' = <<[item |-> it, added |-> it.k = "call"]>>
                   /\ dpend' = IF it.k = "call"
                                 THEN [dpend EXCEPT ![dlive] = @ \cup {it.id}]
                                 ELSE dpend
                   /\ UNCHANGED ddel
    /\ UNCHANGED <<dlive, dstarted, ddisp, drun, cst, dfin, dmeta, dacc, wvars, avars>>
    /\ UNCHANGED <<dst, handled, qdrop, unlisted, bst, bdel>>

\* stream.Send. It blocks while the wire is full (flow control).
SendD ==
    /\ dlive # 0 /\ dhand # <<>>
    /\ ~broken[dlive] /\ Len(c2s[dlive]) < WireCap
    /\ c2s' = [c2s EXCEPT ![dlive] = Append(@, dhand[1].item)]
    /\ dhand' = <<>>
    /\ UNCHANGED <<dq, dlive, dstarted, ddisp, drun, dpend, cst, dfin, dmeta, dacc>>
    /\ UNCHANGED <<s2c, broken, faults, goaways, avars, cvars>>

\* session.handle: a reply goes to its pending call, a request to the
\* dispatcher. Receiving waits while the dispatcher buffer is full.
\* After a break, D still receives the frames that the break did not lose,
\* until the session ends. A draining stream keeps receiving.
\* The acceptance frame models the gorums-accepted header.
RecvD(s) ==
    /\ cst[s] \in {"open", "draining"} /\ s2c[s] # <<>>
    /\ LET f == Head(s2c[s]) IN
         CASE f.k = "acc" ->
                /\ s2c' = [s2c EXCEPT ![s] = Tail(@)]
                /\ dacc' = [dacc EXCEPT ![s] = TRUE]
                /\ UNCHANGED <<ddisp, dpend, dst, ddel>>
           [] f.k = "reply" ->
                /\ s2c' = [s2c EXCEPT ![s] = Tail(@)]
                /\ IF f.id \in dpend[s]
                     THEN /\ dpend' = [dpend EXCEPT ![s] = @ \ {f.id}]
                          /\ dst' = Mark(dst, {f.id}, "ok")
                          /\ ddel' = Count(ddel, {f.id})
                     ELSE UNCHANGED <<dpend, dst, ddel>>   \* unmatched; dropped
                /\ UNCHANGED <<ddisp, dacc>>
           [] OTHER ->
                /\ Len(ddisp) < RecvBuf
                /\ s2c' = [s2c EXCEPT ![s] = Tail(@)]
                /\ ddisp' = Append(ddisp, f.id)
                /\ UNCHANGED <<dpend, dst, ddel, dacc>>
    /\ UNCHANGED <<dq, dhand, dlive, dstarted, drun, cst, dfin, dmeta>>
    /\ UNCHANGED <<c2s, broken, faults, goaways, avars, handled, qdrop, unlisted, bst, bdel>>

\* The dispatcher runs one handler at a time.
HandleD ==
    /\ drun = <<>> /\ ddisp # <<>>
    /\ drun' = <<Head(ddisp)>>
    /\ ddisp' = Tail(ddisp)
    /\ UNCHANGED <<dq, dhand, dlive, dstarted, dpend, cst, dfin, dmeta, dacc>>
    /\ UNCHANGED <<wvars, avars, cvars>>

\* endpoint.reply: tryPush drops the reply when the queue is full.
ReplyD ==
    /\ drun # <<>>
    /\ \/ /\ Len(dq) < QCap
          /\ dq' = Append(dq, Item("reply", drun[1]))
       \/ /\ Len(dq) >= QCap /\ ~BlockingReplies
          /\ UNCHANGED dq
    /\ drun' = <<>>
    /\ UNCHANGED <<dhand, dlive, dstarted, ddisp, dpend, cst, dfin, dmeta, dacc>>
    /\ UNCHANGED <<wvars, avars, cvars>>

\* The server sends a GOAWAY on s's connection, as under MaxConnectionAge,
\* and watchGoAway drains s. D sends nothing more on s and half-closes it,
\* but keeps receiving on it. The next request goes on a new stream. A
\* request in hand that is not yet pending goes there too; sendLoop returns it.
\* A connection that breaks also leaves Ready, so s may already be broken.
GoAwayD ==
    /\ goaways < MaxGoAways /\ dlive # 0
    /\ IF dhand = <<>> THEN TRUE ELSE ~dhand[1].added
    /\ cst' = [cst EXCEPT ![dlive] = "draining"]
    /\ dfin' = [dfin EXCEPT ![dlive] = TRUE]
    /\ dlive' = 0
    /\ goaways' = goaways + 1
    /\ UNCHANGED <<dq, dhand, dstarted, ddisp, drun, dpend, dmeta, dacc>>
    /\ UNCHANGED <<c2s, s2c, broken, faults, avars, cvars>>

\* The session ends after its stream broke: session.end and failPending.
\* Current code: every pending call fails with ErrStreamDown.
\* Bug variant: pending calls go back to the tail of the send queue while it
\* has space; the rest fail with ErrSendQueueFull.
\* A call in hand on the live stream is pending, so its send fails and it is
\* handled as above. Any other item in hand goes to the next stream. Frames
\* that D has not yet received on s are lost.
EndD(s) ==
    /\ cst[s] \in {"open", "draining"} /\ broken[s]
    /\ LET ps     == SetToSeq(dpend[s])
           n      == IF Requeue THEN Min(Len(ps), QCap - Len(dq)) ELSE 0
           failed == {ps[i] : i \in (n + 1)..Len(ps)}
       IN /\ dq' = dq \o [i \in 1..n |-> Item("call", ps[i])]
          /\ dst' = Mark(dst, failed, "down")
          /\ ddel' = Count(ddel, failed)
    /\ dpend' = [dpend EXCEPT ![s] = {}]
    /\ cst' = [cst EXCEPT ![s] = "ended"]
    /\ dlive' = IF dlive = s THEN 0 ELSE dlive
    /\ dhand' = IF dlive = s /\ dhand # <<>> /\ dhand[1].added THEN <<>> ELSE dhand
    /\ s2c' = [s2c EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dstarted, ddisp, drun, dfin, dmeta, dacc>>
    /\ UNCHANGED <<c2s, broken, faults, goaways, avars, handled, qdrop, unlisted, bst, bdel>>

\* A ended the draining stream s without a break, and D has received what A
\* sent on it. A call still pending lost its reply and fails with
\* ErrStreamDown.
DrainEndD(s) ==
    /\ cst[s] = "draining" /\ sfin[s] /\ s2c[s] = <<>>
    /\ dst' = Mark(dst, dpend[s], "down")
    /\ ddel' = Count(ddel, dpend[s])
    /\ dpend' = [dpend EXCEPT ![s] = {}]
    /\ cst' = [cst EXCEPT ![s] = "ended"]
    /\ UNCHANGED <<dq, dhand, dlive, dstarted, ddisp, drun, dfin, dmeta, dacc>>
    /\ UNCHANGED <<wvars, avars, handled, qdrop, unlisted, bst, bdel>>

-----------------------------------------------------------------------------
(* The network                                                             *)

\* The stream breaks. Frames in flight on it are lost.
\* LateFrames: only a suffix is lost. The receiver still gets the prefix,
\* which models data in transit or in buffers when the sender gives up, as
\* after a keepalive timeout.
Break(s) ==
    /\ faults < MaxFaults /\ cst[s] \in {"open", "draining"} /\ ~broken[s]
    /\ broken' = [broken EXCEPT ![s] = TRUE]
    /\ IF LateFrames
         THEN \E i \in 0..Len(c2s[s]), j \in 0..Len(s2c[s]) :
                /\ c2s' = [c2s EXCEPT ![s] = SubSeq(@, 1, i)]
                /\ s2c' = [s2c EXCEPT ![s] = SubSeq(@, 1, j)]
         ELSE /\ c2s' = [c2s EXCEPT ![s] = <<>>]
              /\ s2c' = [s2c EXCEPT ![s] = <<>>]
    /\ faults' = faults + 1
    /\ UNCHANGED <<goaways, dvars, avars, cvars>>

-----------------------------------------------------------------------------
(* A: InboundManager.registerPeer, Node.attachStream, InboundChannel       *)

\* Clears the inbound channels of the streams in P and fails A's calls on
\* them: pending calls with ErrStreamDown, queued calls with ErrNodeClosed.
\* The dispatchers discard their queued requests. A handler that already
\* runs keeps running; ReplyS drops its reply.
CloseAll(P) ==
    LET F == UNION {apend[p] \cup CallsIn(aq[p]) : p \in P} IN
    /\ bst' = Mark(bst, F, "err")
    /\ bdel' = Count(bdel, F)
    /\ apend' = [t \in Streams |-> IF t \in P THEN {} ELSE apend[t]]
    /\ aq' = [t \in Streams |-> IF t \in P THEN <<>> ELSE aq[t]]
    /\ ahand' = [t \in Streams |-> IF t \in P THEN <<>> ELSE ahand[t]]
    /\ adisp' = [t \in Streams |-> IF t \in P THEN <<>> ELSE adisp[t]]
    /\ UNCHANGED arun

\* The older streams that A keeps when it attaches s: with FIFOGate =
\* "drain", the streams that D listed as draining when it opened s.
Kept(s) == IF FIFOGate = "drain" THEN dmeta[s] ELSE {}

\* A has seen the start of stream t.
Seen(t) == sst[t] \notin {"none", "lost"}

\* Supersede: A refuses s if it has seen a newer stream that does not keep s.
Stale(s) == Supersede /\ \E t \in Streams : t > s /\ Seen(t) /\ s \notin Kept(t)

\* The server runs NodeStream for stream s and attaches a channel. The start
\* of a stream may reach the server after the stream broke or drained, so A
\* can attach streams out of D's order.
\* Supersede: A refuses a stale stream at once, and ends it. Attaching s
\* closes each older channel that s does not keep, so A starts no more
\* requests from it. A kept stream that A attaches after a newer one takes
\* the slot only if the slot holds no attached channel.
\* FIFOGate = "drain": after attaching s, A sends its acceptance on s.
AcceptS(s) ==
    /\ sst[s] = "none" /\ cst[s] # "none"
    /\ IF Stale(s)
         THEN /\ sst' = [sst EXCEPT ![s] = "detached"]
              /\ sfin' = [sfin EXCEPT ![s] = TRUE]
              /\ c2s' = [c2s EXCEPT ![s] = <<>>]
              /\ UNCHANGED <<s2c, aq, ahand, adisp, arun, apend, live, slot, bst, bdel>>
         ELSE LET newer == Supersede /\ \E t \in Streams : t > s /\ Seen(t)
                  old   == CASE AttachClosesPrior -> (IF slot # 0 THEN {slot} ELSE {})
                             [] Supersede ->
                                  {t \in Streams : t < s /\ sst[t] \in {"attached", "sealed"}
                                                         /\ t \notin Kept(s)}
                             [] OTHER -> {}
              IN /\ sst' = [t \in Streams |-> IF t = s THEN "attached"
                                              ELSE IF t \in old THEN "closed"
                                              ELSE sst[t]]
                 /\ CloseAll(old)
                 /\ live' = CASE AttachClosesPrior -> live
                              [] newer             -> <<s>> \o live
                              [] Supersede         -> Append(SelectSeq(live, LAMBDA t : t \notin old), s)
                              [] OTHER             -> Append(live, s)
                 /\ slot' = IF newer /\ slot # 0 /\ sst[slot] = "attached" THEN slot ELSE s
                 /\ s2c' = IF FIFOGate = "drain" /\ ~broken[s]
                             THEN [s2c EXCEPT ![s] = <<Item("acc", 0)>>]
                             ELSE s2c
                 /\ UNCHANGED <<c2s, sfin>>
    /\ UNCHANGED <<dvars, broken, faults, goaways, dst, ddel, handled, qdrop, unlisted>>

\* The cleanup after Serve returns detaches the channel of s.
\* Current code: the slot falls back to the latest remaining live channel.
\* Bug variant: the slot is cleared only if it holds this channel.
DetachS(s) ==
    /\ sst' = [sst EXCEPT ![s] = "detached"]
    /\ CloseAll({s})
    /\ LET rest == SelectSeq(live, LAMBDA t : t # s) IN
         IF AttachClosesPrior
           THEN /\ slot' = IF slot = s THEN 0 ELSE slot
                /\ UNCHANGED live
           ELSE /\ live' = rest
                /\ slot' = IF rest = <<>> THEN 0 ELSE rest[Len(rest)]
    /\ c2s' = [c2s EXCEPT ![s] = <<>>]

\* LostStarts: the start of a broken stream never reaches A's server.
LoseStart(s) ==
    /\ LostStarts /\ broken[s] /\ sst[s] = "none"
    /\ sst' = [sst EXCEPT ![s] = "lost"]
    /\ c2s' = [c2s EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, s2c, broken, faults, goaways>>
    /\ UNCHANGED <<aq, ahand, adisp, arun, apend, live, slot, sfin, cvars>>

\* Serve returns after the stream broke.
ServerEndS(s) ==
    /\ sst[s] \in {"attached", "sealed", "closed"} /\ broken[s]
    /\ DetachS(s)
    /\ UNCHANGED <<dvars, s2c, broken, faults, goaways, sfin, dst, ddel, handled, qdrop, unlisted>>

\* Serve reads the end of a stream that D half-closed. It fails A's calls
\* that await D's replies, since D can no longer send them on s. The sealed
\* dispatcher still runs the queued requests.
SealS(s) ==
    /\ sst[s] = "attached" /\ dfin[s] /\ c2s[s] = <<>> /\ ~broken[s]
    /\ sst' = [sst EXCEPT ![s] = "sealed"]
    /\ bst' = Mark(bst, apend[s], "err")
    /\ bdel' = Count(bdel, apend[s])
    /\ apend' = [apend EXCEPT ![s] = {}]
    /\ UNCHANGED <<dvars, wvars, aq, ahand, adisp, arun, live, slot, sfin>>
    /\ UNCHANGED <<dst, ddel, handled, qdrop, unlisted>>

\* Serve returns nil without a break and ends s: a sealed channel once its
\* handlers have returned and its replies are sent, and a closed channel once
\* it reads the end of the stream.
FinishS(s) ==
    /\ ~broken[s]
    /\ \/ /\ sst[s] = "sealed" /\ adisp[s] = <<>> /\ arun[s] = <<>>
          /\ aq[s] = <<>> /\ ahand[s] = <<>>
       \/ sst[s] = "closed" /\ dfin[s] /\ c2s[s] = <<>>
    /\ DetachS(s)
    /\ sfin' = [sfin EXCEPT ![s] = TRUE]
    /\ UNCHANGED <<dvars, s2c, broken, faults, goaways, dst, ddel, handled, qdrop, unlisted>>

\* The receive side of the inbound session. A closed channel's session still
\* receives, but it drops requests. After a break, A still receives the
\* frames that the break did not lose, until Serve returns.
RecvS(s) ==
    /\ sst[s] \in {"attached", "closed"} /\ c2s[s] # <<>>
    /\ LET f == Head(c2s[s]) IN
         IF f.k = "reply"
           THEN /\ c2s' = [c2s EXCEPT ![s] = Tail(@)]
                /\ IF f.id \in apend[s]
                     THEN /\ apend' = [apend EXCEPT ![s] = @ \ {f.id}]
                          /\ bst' = Mark(bst, {f.id}, "ok")
                          /\ bdel' = Count(bdel, {f.id})
                     ELSE UNCHANGED <<apend, bst, bdel>>
                /\ UNCHANGED adisp
           ELSE IF sst[s] = "closed"
             THEN /\ c2s' = [c2s EXCEPT ![s] = Tail(@)]
                  /\ UNCHANGED <<adisp, apend, bst, bdel>>
             ELSE /\ Len(adisp[s]) < RecvBuf
                  /\ c2s' = [c2s EXCEPT ![s] = Tail(@)]
                  /\ adisp' = [adisp EXCEPT ![s] = Append(@, f.id)]
                  /\ UNCHANGED <<apend, bst, bdel>>
    /\ UNCHANGED <<dvars, s2c, broken, faults, goaways>>
    /\ UNCHANGED <<sst, aq, ahand, arun, live, slot, sfin, dst, ddel, handled, qdrop, unlisted>>

\* FIFOGate = "idle": A starts a handler on s only while each older channel
\* that A knows is detached, or has no queued or running request.
IdleGate(s) ==
    \A t \in Streams : t < s =>
        \/ sst[t] \in {"none", "detached", "lost"}
        \/ adisp[t] = <<>> /\ arun[t] = <<>>

\* A has run everything it will run from stream t.
Finished(t) ==
    \/ sst[t] \in {"closed", "detached"}
    \/ sst[t] = "sealed" /\ adisp[t] = <<>>

\* Supersede: A starts a handler on s only after the handlers of older
\* channels have returned, and after each kept stream has finished.
SupersedeGate(s) ==
    \A t \in Streams : t < s =>
        /\ arun[t] = <<>>
        /\ t \in Kept(s) => Finished(t)

HandleS(s) ==
    /\ sst[s] \in {"attached", "sealed"} /\ arun[s] = <<>> /\ adisp[s] # <<>>
    /\ FIFOGate = "idle" => IdleGate(s)
    /\ Supersede => SupersedeGate(s)
    /\ arun' = [arun EXCEPT ![s] = <<Head(adisp[s])>>]
    /\ adisp' = [adisp EXCEPT ![s] = Tail(@)]
    /\ handled' = Count(handled, {Head(adisp[s])})
    /\ UNCHANGED <<dvars, wvars, sst, aq, ahand, apend, live, slot, sfin>>
    /\ UNCHANGED <<dst, ddel, qdrop, unlisted, bst, bdel>>

\* The handler returns. Its reply is dropped if the channel is closed or
\* detached, or if the send queue is full.
ReplyS(s) ==
    /\ arun[s] # <<>>
    /\ IF sst[s] \in {"attached", "sealed"}
         THEN \/ /\ Len(aq[s]) < QCap
                 /\ aq' = [aq EXCEPT ![s] = Append(@, Item("reply", arun[s][1]))]
                 /\ UNCHANGED qdrop
              \/ /\ Len(aq[s]) >= QCap /\ ~BlockingReplies
                 /\ qdrop' = qdrop \cup {arun[s][1]}
                 /\ UNCHANGED aq
         ELSE UNCHANGED <<aq, qdrop>>
    /\ arun' = [arun EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, wvars, sst, ahand, adisp, apend, live, slot, sfin>>
    /\ UNCHANGED <<dst, ddel, handled, unlisted, bst, bdel>>

TakeS(s) ==
    /\ sst[s] \in {"attached", "sealed"} /\ ahand[s] = <<>> /\ aq[s] # <<>>
    /\ LET it == Head(aq[s]) IN
         /\ aq' = [aq EXCEPT ![s] = Tail(@)]
         /\ IF it.k = "call" /\ bst[it.id] = "gone"
              THEN /\ bdel' = Count(bdel, {it.id})
                   /\ UNCHANGED <<ahand, apend>>
              ELSE /\ ahand' = [ahand EXCEPT ![s] =
                                  <<[item |-> it, added |-> it.k = "call"]>>]
                   /\ apend' = IF it.k = "call"
                                 THEN [apend EXCEPT ![s] = @ \cup {it.id}]
                                 ELSE apend
                   /\ UNCHANGED bdel
    /\ UNCHANGED <<dvars, wvars, sst, adisp, arun, live, slot, sfin>>
    /\ UNCHANGED <<dst, ddel, handled, qdrop, unlisted, bst>>

SendS(s) ==
    /\ sst[s] \in {"attached", "sealed"} /\ ahand[s] # <<>>
    /\ ~broken[s] /\ Len(s2c[s]) < WireCap
    /\ s2c' = [s2c EXCEPT ![s] = Append(@, ahand[s][1].item)]
    /\ ahand' = [ahand EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, c2s, broken, faults, goaways>>
    /\ UNCHANGED <<sst, aq, adisp, arun, apend, live, slot, sfin, cvars>>

-----------------------------------------------------------------------------
(* Specification                                                           *)

Init ==
    /\ dq = <<>> /\ dhand = <<>> /\ dlive = 0 /\ dstarted = FALSE
    /\ ddisp = <<>> /\ drun = <<>>
    /\ dpend = [s \in Streams |-> {}]
    /\ cst = [s \in Streams |-> "none"]
    /\ dfin = [s \in Streams |-> FALSE]
    /\ dmeta = [s \in Streams |-> {}]
    /\ dacc = [s \in Streams |-> FALSE]
    /\ c2s = [s \in Streams |-> <<>>]
    /\ s2c = [s \in Streams |-> <<>>]
    /\ broken = [s \in Streams |-> FALSE]
    /\ faults = 0 /\ goaways = 0
    /\ sst = [s \in Streams |-> "none"]
    /\ aq = [s \in Streams |-> <<>>]
    /\ ahand = [s \in Streams |-> <<>>]
    /\ adisp = [s \in Streams |-> <<>>]
    /\ arun = [s \in Streams |-> <<>>]
    /\ apend = [s \in Streams |-> {}]
    /\ live = <<>> /\ slot = 0
    /\ sfin = [s \in Streams |-> FALSE]
    /\ dst = [c \in DCalls |-> "idle"]
    /\ ddel = [c \in DCalls |-> 0]
    /\ handled = [c \in DCalls |-> 0]
    /\ qdrop = {} /\ unlisted = {}
    /\ bst = [b \in BCalls |-> "idle"]
    /\ bdel = [b \in BCalls |-> 0]

\* Every caller has issued its call, and no handler has work.
Quiescent ==
    /\ \A c \in DCalls : dst[c] # "idle"
    /\ \A b \in BCalls : bst[b] # "idle"
    /\ drun = <<>> /\ ddisp = <<>>
    /\ \A s \in Streams : arun[s] = <<>> /\ adisp[s] = <<>>

\* Lets a quiescent system stutter, so that TLC reports as a deadlock only
\* a state where work remains and no step can take it.
Done == Quiescent /\ UNCHANGED vars

Next ==
    \/ \E c \in DCalls : IssueD(c) \/ CancelD(c)
    \/ \E b \in BCalls : IssueB(b) \/ CancelB(b)
    \/ OpenD \/ TakeD \/ SendD \/ HandleD \/ ReplyD \/ GoAwayD
    \/ \E s \in Streams :
         \/ RecvD(s) \/ EndD(s) \/ DrainEndD(s) \/ Break(s) \/ LoseStart(s)
         \/ AcceptS(s) \/ ServerEndS(s) \/ SealS(s) \/ FinishS(s) \/ RecvS(s)
         \/ HandleS(s) \/ ReplyS(s) \/ TakeS(s) \/ SendS(s)
    \/ Done

\* The runtime's goroutines are fair. Callers, the network, and GOAWAYs are not.
Fairness ==
    /\ WF_vars(OpenD) /\ WF_vars(TakeD) /\ WF_vars(SendD)
    /\ WF_vars(HandleD) /\ WF_vars(ReplyD)
    /\ \A s \in Streams :
         /\ WF_vars(RecvD(s)) /\ WF_vars(EndD(s)) /\ WF_vars(DrainEndD(s))
         /\ WF_vars(AcceptS(s)) /\ WF_vars(ServerEndS(s)) /\ WF_vars(RecvS(s))
         /\ WF_vars(SealS(s)) /\ WF_vars(FinishS(s))
         /\ WF_vars(HandleS(s)) /\ WF_vars(ReplyS(s))
         /\ WF_vars(TakeS(s)) /\ WF_vars(SendS(s))

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* Properties                                                              *)

Item_(ids) == [k : {"call", "reply"}, id : ids]
Hand(ids) == {<<>>} \cup {<<h>> : h \in [item : Item_(ids), added : BOOLEAN]}
Run(ids) == {<<>>} \cup {<<x>> : x \in ids}
Status == {"idle", "active", "ok", "err", "down", "gone"}

TypeOK ==
    /\ dq \in Seq(Item_(DCalls \cup BCalls)) /\ Len(dq) <= QCap
    /\ dhand \in Hand(DCalls \cup BCalls)
    /\ dlive \in {0} \cup Streams
    /\ ddisp \in Seq(BCalls) /\ Len(ddisp) <= RecvBuf
    /\ drun \in Run(BCalls)
    /\ dpend \in [Streams -> SUBSET DCalls]
    /\ cst \in [Streams -> {"none", "open", "draining", "ended"}]
    /\ dmeta \in [Streams -> SUBSET Streams]
    /\ \A s \in Streams : Len(c2s[s]) <= WireCap /\ Len(s2c[s]) <= WireCap
    /\ faults \in 0..MaxFaults /\ goaways \in 0..MaxGoAways
    /\ sst \in [Streams -> {"none", "attached", "sealed", "closed", "detached", "lost"}]
    /\ \A s \in Streams : Len(aq[s]) <= QCap /\ Len(adisp[s]) <= RecvBuf
    /\ apend \in [Streams -> SUBSET BCalls]
    /\ slot \in {0} \cup Streams
    /\ qdrop \subseteq DCalls /\ unlisted \subseteq DCalls
    /\ dacc \in [Streams -> BOOLEAN]
    /\ dst \in [DCalls -> Status] /\ bst \in [BCalls -> Status]

\* The runtime delivers at most one response to each two-way call.
AtMostOnceDelivery ==
    /\ \A c \in DCalls : ddel[c] <= 1
    /\ \A b \in BCalls : bdel[b] <= 1

\* A call is in at most one place: one send queue or one pending table.
NoDuplicateCall ==
    /\ \A c \in DCalls :
         Cardinality({i \in DOMAIN dq : dq[i] = Item("call", c)})
           + Cardinality({s \in Streams : c \in dpend[s]}) <= 1
    /\ \A b \in BCalls :
         Cardinality({<<s, i>> \in Streams \X (1..QCap) :
                        i <= Len(aq[s]) /\ aq[s][i] = Item("call", b)})
           + Cardinality({s \in Streams : b \in apend[s]}) <= 1

\* While A serves a stream that D still sends on and the network has not
\* broken, the shared node's slot holds an attached channel, so A's calls to D
\* can reach D.
SlotTracksLive ==
    (\E s \in Streams : sst[s] \in {"attached", "closed"} /\ ~dfin[s] /\ ~broken[s])
        => slot # 0 /\ sst[slot] = "attached"

\* D opens a stream only after every older stream ended or started draining.
OlderStreamsEnded ==
    \A s, t \in Streams : s < t /\ cst[t] # "none" => cst[s] \in {"draining", "ended"}

\* After the last fault, A's shared node gets a working stream to D and keeps it.
BorrowerRecovers == <>[](slot # 0 /\ ~broken[slot])

\* Without a stream break, a call fails with ErrStreamDown only if A dropped
\* its reply because A's send queue was full. A GOAWAY loses no call.
DrainSafe == faults = 0 => \A c \in DCalls : dst[c] = "down" => c \in qdrop

\* DrainSafe for the streams that D listed as draining. A call may also fail
\* if it was pending on a draining stream whose acceptance D had not received
\* when it opened the next stream.
ListedDrainSafe ==
    faults = 0 => \A c \in DCalls : dst[c] = "down" => c \in qdrop \cup unlisted

\* Contract: A's handler runs each call at most once.
AtMostOnceExecution == \A c \in DCalls : handled[c] <= 1

\* Contract: A's handlers run D's calls one at a time.
OneAtATime == Cardinality({s \in Streams : arun[s] # <<>>}) <= 1

\* Call symmetry for safety checks without faults. SetToSeq in EndD breaks
\* symmetry, so do not use it when MaxFaults > 0, nor with liveness.
CallSymmetry == Permutations(DCalls) \cup Permutations(BCalls)

=============================================================================
