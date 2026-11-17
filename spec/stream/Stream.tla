------------------------------- MODULE Stream -------------------------------
(***************************************************************************)
(* A model of the Gorums stream layer for one peer pair under stream       *)
(* deduplication.                                                          *)
(*                                                                         *)
(* D is the lower-ID peer. It is the only dialer. Its OutboundChannel      *)
(* opens one stream at a time and sends D's calls on it.                   *)
(*                                                                         *)
(* A is the higher-ID peer. Its server accepts D's streams. Each accepted  *)
(* stream gets an InboundChannel, which is attached to A's known-peer node *)
(* for D. A's outbound node for D is born shared: it borrows that node's   *)
(* channel slot, so A's calls to D travel on D's stream as                 *)
(* server-initiated requests.                                              *)
(*                                                                         *)
(* Code map:                                                               *)
(*   dq, dhand, dlive, OpenD, TakeD, SendD, EndD                           *)
(*        internal/stream/outbound.go (run) and session.go (sendLoop, end) *)
(*   dpend, apend    session.pending (pendingCalls)                        *)
(*   ddisp, adisp    endpoint.requests (dispatcher, RecvBufferSize)        *)
(*   ReplyD, ReplyS  endpoint.reply                                        *)
(*   AcceptS, ServerEndS, live, slot                                       *)
(*        internal/conn/node.go (attachStream, liveChannels)               *)
(*   IssueB          Transport.Enqueue on the shared transport             *)
(*                                                                         *)
(* Not modeled: refusals and their backoff, GOAWAY and Drain, streaming    *)
(* calls, one-way calls, ResourceExhausted, LocalChannel, and channel      *)
(* Close by the application.                                               *)
(***************************************************************************)
EXTENDS Naturals, Sequences, FiniteSets, TLC

CONSTANTS
    DCalls,             \* calls that D sends to A
    BCalls,             \* calls that A's shared node sends to D
    QCap,               \* send queue capacity, on both sides
    WireCap,            \* frames in flight per stream direction (flow control)
    RecvBuf,            \* dispatcher buffer (RecvBufferSize)
    MaxFaults,          \* number of stream breaks the network may inject
    AllowCancel,        \* callers may give up (context deadline)
    \* Bug switches. All FALSE models the current code.
    LazyReconnect,      \* D reopens a lost stream only when it has a request
    AttachClosesPrior,  \* attach closes the prior channel; detach clears the slot only if it holds its own channel
    BlockingReplies,    \* a handler's reply waits for send queue space
    \* Design switches. All off models the current code.
    NoRequeue,          \* a lost stream fails its pending calls; it does not requeue them
    FIFOGate,           \* "none", "idle", or "supersede"; see AcceptS and HandleS
    \* Network switch.
    LateFrames          \* a break loses only a suffix of the frames in flight

ASSUME QCap >= 1 /\ WireCap >= 1 /\ RecvBuf >= 1 /\ MaxFaults \in Nat
ASSUME FIFOGate \in {"none", "idle", "supersede"}
ASSUME DCalls \cap BCalls = {}

\* Every stream ends only when it breaks, so D opens at most MaxFaults+1.
Streams == 1..(MaxFaults + 1)

VARIABLES
    \* D, the dialer.
    dq,        \* send queue: a sequence of items
    dhand,     \* <<>> or <<[item, added]>>: an item taken from dq, not yet sent
    dlive,     \* the stream of the live session, or 0
    dstarted,  \* the channel has made its first connect
    ddisp,     \* requests from A waiting for D's handler
    drun,      \* <<>> or <<b>>: the request D's handler is running
    dpend,     \* dpend[s]: D's calls pending on stream s
    cst,       \* cst[s]: D's view of stream s: "none", "open", or "ended"
    \* The network.
    c2s,       \* c2s[s]: frames in flight from D to A on stream s
    s2c,       \* s2c[s]: frames in flight from A to D on stream s
    broken,    \* broken[s]: the network broke stream s
    faults,    \* number of breaks so far
    \* A, the acceptor.
    sst,       \* sst[s]: A's inbound channel for s: "none", "attached", "closed", or "detached"
    aq,        \* aq[s]: the inbound channel's send queue
    ahand,     \* ahand[s]: as dhand
    adisp,     \* adisp[s]: requests from D waiting for A's handler
    arun,      \* arun[s]: as drun
    apend,     \* apend[s]: A's calls pending on stream s
    live,      \* Node.liveChannels for D, in attach order
    slot,      \* the stream behind the node's channelRef, or 0 for nil
    \* Calls.
    dst,       \* dst[c]: "idle", "active", "ok", "err", or "gone"
    ddel,      \* ddel[c]: responses the runtime delivered to c's caller
    handled,   \* handled[c]: times A's handler ran c
    bst,       \* bst[b]: as dst
    bdel       \* bdel[b]: as ddel

dvars == <<dq, dhand, dlive, dstarted, ddisp, drun, dpend, cst>>
wvars == <<c2s, s2c, broken, faults>>
avars == <<sst, aq, ahand, adisp, arun, apend, live, slot>>
cvars == <<dst, ddel, handled, bst, bdel>>
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
    /\ UNCHANGED <<dhand, dlive, dstarted, ddisp, drun, dpend, cst>>
    /\ UNCHANGED <<wvars, avars, ddel, handled, bst, bdel>>

\* A's call b: Transport.Enqueue on the shared transport, which loads the slot.
IssueB(b) ==
    /\ bst[b] = "idle"
    /\ IF slot # 0 /\ sst[slot] = "attached" /\ Len(aq[slot]) < QCap
         THEN /\ aq' = [aq EXCEPT ![slot] = Append(@, Item("call", b))]
              /\ bst' = [bst EXCEPT ![b] = "active"]
         ELSE /\ bst' = [bst EXCEPT ![b] = "err"]      \* ErrStreamDown or ErrSendQueueFull
              /\ UNCHANGED aq
    /\ UNCHANGED <<dvars, wvars, sst, ahand, adisp, arun, apend, live, slot>>
    /\ UNCHANGED <<dst, ddel, handled, bdel>>

\* A caller's context ends. The runtime does not learn of it until it next
\* touches the call.
CancelD(c) ==
    /\ AllowCancel /\ dst[c] = "active"
    /\ dst' = [dst EXCEPT ![c] = "gone"]
    /\ UNCHANGED <<dvars, wvars, avars, ddel, handled, bst, bdel>>

CancelB(b) ==
    /\ AllowCancel /\ bst[b] = "active"
    /\ bst' = [bst EXCEPT ![b] = "gone"]
    /\ UNCHANGED <<dvars, wvars, avars, dst, ddel, handled, bdel>>

-----------------------------------------------------------------------------
(* D: OutboundChannel.run, session.sendLoop, session.receive               *)

\* run opens a stream at startup. After a lost stream it opens the next one at
\* once if it holds a request, and otherwise under eager reconnect, or when a
\* request arrives.
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
    /\ dstarted' = TRUE
    /\ UNCHANGED <<dq, dhand, ddisp, drun, dpend, wvars, avars, cvars>>

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
    /\ UNCHANGED <<dlive, dstarted, ddisp, drun, cst, wvars, avars>>
    /\ UNCHANGED <<dst, handled, bst, bdel>>

\* stream.Send. It blocks while the wire is full (flow control).
SendD ==
    /\ dlive # 0 /\ dhand # <<>>
    /\ ~broken[dlive] /\ Len(c2s[dlive]) < WireCap
    /\ c2s' = [c2s EXCEPT ![dlive] = Append(@, dhand[1].item)]
    /\ dhand' = <<>>
    /\ UNCHANGED <<dq, dlive, dstarted, ddisp, drun, dpend, cst>>
    /\ UNCHANGED <<s2c, broken, faults, avars, cvars>>

\* session.handle: a reply goes to its pending call, a request to the
\* dispatcher. Receiving waits while the dispatcher buffer is full.
\* After a break, D still receives the frames that the break did not lose,
\* until the session ends.
RecvD(s) ==
    /\ cst[s] = "open" /\ s2c[s] # <<>>
    /\ LET f == Head(s2c[s]) IN
         IF f.k = "reply"
           THEN /\ s2c' = [s2c EXCEPT ![s] = Tail(@)]
                /\ IF f.id \in dpend[s]
                     THEN /\ dpend' = [dpend EXCEPT ![s] = @ \ {f.id}]
                          /\ dst' = Mark(dst, {f.id}, "ok")
                          /\ ddel' = Count(ddel, {f.id})
                     ELSE UNCHANGED <<dpend, dst, ddel>>   \* unmatched; dropped
                /\ UNCHANGED ddisp
           ELSE /\ Len(ddisp) < RecvBuf
                /\ s2c' = [s2c EXCEPT ![s] = Tail(@)]
                /\ ddisp' = Append(ddisp, f.id)
                /\ UNCHANGED <<dpend, dst, ddel>>
    /\ UNCHANGED <<dq, dhand, dlive, dstarted, drun, cst>>
    /\ UNCHANGED <<c2s, broken, faults, avars, handled, bst, bdel>>

\* The dispatcher runs one handler at a time.
HandleD ==
    /\ drun = <<>> /\ ddisp # <<>>
    /\ drun' = <<Head(ddisp)>>
    /\ ddisp' = Tail(ddisp)
    /\ UNCHANGED <<dq, dhand, dlive, dstarted, dpend, cst, wvars, avars, cvars>>

\* endpoint.reply: tryPush drops the reply when the queue is full.
ReplyD ==
    /\ drun # <<>>
    /\ \/ /\ Len(dq) < QCap
          /\ dq' = Append(dq, Item("reply", drun[1]))
       \/ /\ Len(dq) >= QCap /\ ~BlockingReplies
          /\ UNCHANGED dq
    /\ drun' = <<>>
    /\ UNCHANGED <<dhand, dlive, dstarted, ddisp, dpend, cst, wvars, avars, cvars>>

\* The session ends after its stream broke: session.end and retryOrFail.
\* Current code: pending calls go back to the tail of the send queue while
\* it has space; the rest fail with ErrSendQueueFull.
\* NoRequeue: every pending call fails with ErrStreamDown.
\* A call in hand is pending, so its send fails and it is handled as above.
\* Any other item in hand goes to the next stream. Frames that D has not
\* yet received on s are lost.
EndD(s) ==
    /\ cst[s] = "open" /\ broken[s]
    /\ LET ps     == SetToSeq(dpend[s])
           n      == IF NoRequeue THEN 0 ELSE Min(Len(ps), QCap - Len(dq))
           failed == {ps[i] : i \in (n + 1)..Len(ps)}
       IN /\ dq' = dq \o [i \in 1..n |-> Item("call", ps[i])]
          /\ dst' = Mark(dst, failed, "err")
          /\ ddel' = Count(ddel, failed)
    /\ dpend' = [dpend EXCEPT ![s] = {}]
    /\ cst' = [cst EXCEPT ![s] = "ended"]
    /\ dlive' = IF dlive = s THEN 0 ELSE dlive
    /\ dhand' = IF dhand # <<>> /\ dhand[1].added THEN <<>> ELSE dhand
    /\ s2c' = [s2c EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dstarted, ddisp, drun, c2s, broken, faults, avars>>
    /\ UNCHANGED <<handled, bst, bdel>>

-----------------------------------------------------------------------------
(* The network                                                             *)

\* The stream breaks. Frames in flight on it are lost.
\* LateFrames: only a suffix is lost. The receiver still gets the prefix,
\* which models data in transit or in buffers when the sender gives up, as
\* after a keepalive timeout.
Break(s) ==
    /\ faults < MaxFaults /\ cst[s] = "open" /\ ~broken[s]
    /\ broken' = [broken EXCEPT ![s] = TRUE]
    /\ IF LateFrames
         THEN \E i \in 0..Len(c2s[s]), j \in 0..Len(s2c[s]) :
                /\ c2s' = [c2s EXCEPT ![s] = SubSeq(@, 1, i)]
                /\ s2c' = [s2c EXCEPT ![s] = SubSeq(@, 1, j)]
         ELSE /\ c2s' = [c2s EXCEPT ![s] = <<>>]
              /\ s2c' = [s2c EXCEPT ![s] = <<>>]
    /\ faults' = faults + 1
    /\ UNCHANGED <<dvars, avars, cvars>>

-----------------------------------------------------------------------------
(* A: InboundManager.registerPeer, Node.attachStream, InboundChannel       *)

\* Clears the inbound channel of stream p and fails A's calls on it:
\* pending calls with ErrStreamDown, queued calls with ErrNodeClosed.
\* The dispatcher discards its queued requests. A handler that already runs
\* keeps running; ReplyS drops its reply.
CloseA(p) ==
    LET F == apend[p] \cup CallsIn(aq[p]) IN
    /\ bst' = Mark(bst, F, "err")
    /\ bdel' = Count(bdel, F)
    /\ apend' = [apend EXCEPT ![p] = {}]
    /\ aq' = [aq EXCEPT ![p] = <<>>]
    /\ ahand' = [ahand EXCEPT ![p] = <<>>]
    /\ adisp' = [adisp EXCEPT ![p] = <<>>]
    /\ UNCHANGED arun

\* The server runs NodeStream for stream s and attaches a channel. The start
\* of a stream may reach the server after the stream broke, so A can attach
\* streams out of D's order.
\* FIFOGate = "supersede": D numbers its streams, and A knows the numbers.
\* A refuses a stream older than one it has seen. D has already ended such a
\* stream (OlderStreamsEnded), so the refusal needs no step at D. Attaching
\* a newer stream closes the older channel, so A starts no more requests
\* from it.
AcceptS(s) ==
    /\ sst[s] = "none" /\ cst[s] # "none"
    /\ IF FIFOGate = "supersede" /\ \E t \in Streams : t > s /\ sst[t] # "none"
         THEN /\ sst' = [sst EXCEPT ![s] = "detached"]
              /\ c2s' = [c2s EXCEPT ![s] = <<>>]
              /\ UNCHANGED <<aq, ahand, adisp, arun, apend, live, slot, bst, bdel>>
         ELSE /\ IF (AttachClosesPrior \/ FIFOGate = "supersede") /\ slot # 0
                   THEN /\ sst' = [sst EXCEPT ![s] = "attached", ![slot] = "closed"]
                        /\ CloseA(slot)
                   ELSE /\ sst' = [sst EXCEPT ![s] = "attached"]
                        /\ UNCHANGED <<aq, ahand, adisp, arun, apend, bst, bdel>>
              /\ live' = CASE AttachClosesPrior        -> live
                           [] FIFOGate = "supersede" -> <<s>>
                           [] OTHER                  -> Append(live, s)
              /\ slot' = s
              /\ UNCHANGED c2s
    /\ UNCHANGED <<dvars, s2c, broken, faults, dst, ddel, handled>>

\* Serve returns after the stream broke, and the cleanup detaches the channel.
\* Current code: the slot falls back to the latest remaining live channel.
\* Bug variant: the slot is cleared only if it holds this channel.
ServerEndS(s) ==
    /\ sst[s] \in {"attached", "closed"} /\ broken[s]
    /\ sst' = [sst EXCEPT ![s] = "detached"]
    /\ CloseA(s)
    /\ LET rest == SelectSeq(live, LAMBDA t : t # s) IN
         IF AttachClosesPrior
           THEN /\ slot' = IF slot = s THEN 0 ELSE slot
                /\ UNCHANGED live
           ELSE /\ live' = rest
                /\ slot' = IF rest = <<>> THEN 0 ELSE rest[Len(rest)]
    /\ c2s' = [c2s EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, s2c, broken, faults, dst, ddel, handled>>

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
    /\ UNCHANGED <<dvars, s2c, broken, faults, sst, aq, ahand, arun, live, slot>>
    /\ UNCHANGED <<dst, ddel, handled>>

\* FIFOGate = "idle": D numbers its streams, and A knows the numbers. A starts
\* a handler on s only while each older channel that A knows is detached, or
\* has no queued or running request.
IdleGate(s) ==
    \A t \in Streams : t < s =>
        \/ sst[t] \in {"none", "detached"}
        \/ adisp[t] = <<>> /\ arun[t] = <<>>

HandleS(s) ==
    /\ sst[s] = "attached" /\ arun[s] = <<>> /\ adisp[s] # <<>>
    /\ FIFOGate = "idle" => IdleGate(s)
    /\ arun' = [arun EXCEPT ![s] = <<Head(adisp[s])>>]
    /\ adisp' = [adisp EXCEPT ![s] = Tail(@)]
    /\ handled' = Count(handled, {Head(adisp[s])})
    /\ UNCHANGED <<dvars, wvars, sst, aq, ahand, apend, live, slot>>
    /\ UNCHANGED <<dst, ddel, bst, bdel>>

\* The handler returns. Its reply is dropped if the channel is closed or
\* detached.
ReplyS(s) ==
    /\ arun[s] # <<>>
    /\ IF sst[s] = "attached"
         THEN \/ /\ Len(aq[s]) < QCap
                 /\ aq' = [aq EXCEPT ![s] = Append(@, Item("reply", arun[s][1]))]
              \/ /\ Len(aq[s]) >= QCap /\ ~BlockingReplies
                 /\ UNCHANGED aq
         ELSE UNCHANGED aq
    /\ arun' = [arun EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, wvars, sst, ahand, adisp, apend, live, slot, cvars>>

TakeS(s) ==
    /\ sst[s] = "attached" /\ ahand[s] = <<>> /\ aq[s] # <<>>
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
    /\ UNCHANGED <<dvars, wvars, sst, adisp, arun, live, slot>>
    /\ UNCHANGED <<dst, ddel, handled, bst>>

SendS(s) ==
    /\ sst[s] = "attached" /\ ahand[s] # <<>>
    /\ ~broken[s] /\ Len(s2c[s]) < WireCap
    /\ s2c' = [s2c EXCEPT ![s] = Append(@, ahand[s][1].item)]
    /\ ahand' = [ahand EXCEPT ![s] = <<>>]
    /\ UNCHANGED <<dvars, c2s, broken, faults, sst, aq, adisp, arun, apend>>
    /\ UNCHANGED <<live, slot, cvars>>

-----------------------------------------------------------------------------
(* Specification                                                           *)

Init ==
    /\ dq = <<>> /\ dhand = <<>> /\ dlive = 0 /\ dstarted = FALSE
    /\ ddisp = <<>> /\ drun = <<>>
    /\ dpend = [s \in Streams |-> {}]
    /\ cst = [s \in Streams |-> "none"]
    /\ c2s = [s \in Streams |-> <<>>]
    /\ s2c = [s \in Streams |-> <<>>]
    /\ broken = [s \in Streams |-> FALSE]
    /\ faults = 0
    /\ sst = [s \in Streams |-> "none"]
    /\ aq = [s \in Streams |-> <<>>]
    /\ ahand = [s \in Streams |-> <<>>]
    /\ adisp = [s \in Streams |-> <<>>]
    /\ arun = [s \in Streams |-> <<>>]
    /\ apend = [s \in Streams |-> {}]
    /\ live = <<>> /\ slot = 0
    /\ dst = [c \in DCalls |-> "idle"]
    /\ ddel = [c \in DCalls |-> 0]
    /\ handled = [c \in DCalls |-> 0]
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
    \/ OpenD \/ TakeD \/ SendD \/ HandleD \/ ReplyD
    \/ \E s \in Streams :
         \/ RecvD(s) \/ EndD(s) \/ Break(s)
         \/ AcceptS(s) \/ ServerEndS(s) \/ RecvS(s)
         \/ HandleS(s) \/ ReplyS(s) \/ TakeS(s) \/ SendS(s)
    \/ Done

\* The runtime's goroutines are fair. Callers and the network are not.
Fairness ==
    /\ WF_vars(OpenD) /\ WF_vars(TakeD) /\ WF_vars(SendD)
    /\ WF_vars(HandleD) /\ WF_vars(ReplyD)
    /\ \A s \in Streams :
         /\ WF_vars(RecvD(s)) /\ WF_vars(EndD(s))
         /\ WF_vars(AcceptS(s)) /\ WF_vars(ServerEndS(s)) /\ WF_vars(RecvS(s))
         /\ WF_vars(HandleS(s)) /\ WF_vars(ReplyS(s))
         /\ WF_vars(TakeS(s)) /\ WF_vars(SendS(s))

Spec == Init /\ [][Next]_vars /\ Fairness

-----------------------------------------------------------------------------
(* Properties                                                              *)

Item_(ids) == [k : {"call", "reply"}, id : ids]
Hand(ids) == {<<>>} \cup {<<h>> : h \in [item : Item_(ids), added : BOOLEAN]}
Run(ids) == {<<>>} \cup {<<x>> : x \in ids}
Status == {"idle", "active", "ok", "err", "gone"}

TypeOK ==
    /\ dq \in Seq(Item_(DCalls \cup BCalls)) /\ Len(dq) <= QCap
    /\ dhand \in Hand(DCalls \cup BCalls)
    /\ dlive \in 0..MaxFaults + 1
    /\ ddisp \in Seq(BCalls) /\ Len(ddisp) <= RecvBuf
    /\ drun \in Run(BCalls)
    /\ dpend \in [Streams -> SUBSET DCalls]
    /\ cst \in [Streams -> {"none", "open", "ended"}]
    /\ \A s \in Streams : Len(c2s[s]) <= WireCap /\ Len(s2c[s]) <= WireCap
    /\ faults \in 0..MaxFaults
    /\ sst \in [Streams -> {"none", "attached", "closed", "detached"}]
    /\ \A s \in Streams : Len(aq[s]) <= QCap /\ Len(adisp[s]) <= RecvBuf
    /\ apend \in [Streams -> SUBSET BCalls]
    /\ slot \in 0..MaxFaults + 1
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

\* While A serves a stream that the network has not broken, the shared node's
\* slot holds an attached channel, so A's calls to D can reach D.
SlotTracksLive ==
    (\E s \in Streams : sst[s] \in {"attached", "closed"} /\ ~broken[s])
        => slot # 0 /\ sst[slot] = "attached"

\* D opens a stream only after every older stream ended.
OlderStreamsEnded ==
    \A s, t \in Streams : s < t /\ cst[t] # "none" => cst[s] = "ended"

\* After the last fault, A's shared node gets a working stream to D and keeps it.
BorrowerRecovers == <>[](slot # 0 /\ ~broken[slot])

\* Contract: A's handler runs each call at most once. Fails on the current
\* code; NoRequeue makes it hold.
AtMostOnceExecution == \A c \in DCalls : handled[c] <= 1

\* Contract: A's handlers run D's calls one at a time.
OneAtATime == Cardinality({s \in Streams : arun[s] # <<>>}) <= 1

\* Call symmetry for safety checks without faults. SetToSeq in EndD breaks
\* symmetry, so do not use it when MaxFaults > 0, nor with liveness.
CallSymmetry == Permutations(DCalls) \cup Permutations(BCalls)

=============================================================================
