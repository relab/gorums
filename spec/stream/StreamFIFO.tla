----------------------------- MODULE StreamFIFO -----------------------------
(***************************************************************************)
(* Stream with two history variables, to check the order in which A starts *)
(* D's calls. The contract is FIFO order within each stream. A new stream  *)
(* is an ordering boundary: A may start a call from an older stream after  *)
(* a later call from its replacement.                                      *)
(* SpecF has no fairness; SpecFL adds it, for liveness checks.             *)
(***************************************************************************)
EXTENDS Stream

VARIABLES
    iorder,  \* D's calls in the order that Enqueue admitted them
    elog     \* <<s, c>> for each handler start at A, in start order: call c from stream s

Admitted == {c \in DCalls : dst[c] = "idle" /\ dst'[c] = "active"}
Started  == {<<s, c>> \in Streams \X DCalls : arun[s] = <<>> /\ arun'[s] = <<c>>}

InitF == Init /\ iorder = <<>> /\ elog = <<>>

NextF ==
    /\ Next
    /\ iorder' = iorder \o SetToSeq(Admitted)
    /\ elog' = elog \o SetToSeq(Started)

SpecF == InitF /\ [][NextF]_<<vars, iorder, elog>>

SpecFL == SpecF /\ Fairness

\* The position of call c in D's admission order. D admits a call once.
Pos(c) == CHOOSE i \in DOMAIN iorder : iorder[i] = c

\* Contract: A starts the calls of one stream in the order that D admitted
\* them.
PerStreamFIFO ==
    \A i, j \in DOMAIN elog :
        i < j /\ elog[i][1] = elog[j][1] => Pos(elog[i][2]) <= Pos(elog[j][2])

\* Not a contract: A starts all of D's calls in the order that D admitted
\* them. A call that waits or arrives late on an older stream can start after
\* a later call from the replacement stream.
CrossStreamFIFO ==
    \A i, j \in DOMAIN elog : i < j => Pos(elog[i][2]) <= Pos(elog[j][2])

=============================================================================
