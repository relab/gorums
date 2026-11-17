----------------------------- MODULE StreamFIFO -----------------------------
(***************************************************************************)
(* Stream with two history variables, to check the user guide's            *)
(* contract that messages from one sender run in FIFO order at a server.   *)
(* SpecF has no fairness; SpecFL adds it, for liveness checks.             *)
(***************************************************************************)
EXTENDS Stream

VARIABLES
    iorder,  \* D's calls in the order that Enqueue admitted them
    elog     \* D's calls in the order that A's handler ran them

Admitted == {c \in DCalls : dst[c] = "idle" /\ dst'[c] = "active"}
Ran      == {c \in DCalls : handled'[c] > handled[c]}

InitF == Init /\ iorder = <<>> /\ elog = <<>>

NextF ==
    /\ Next
    /\ iorder' = iorder \o SetToSeq(Admitted)
    /\ elog' = elog \o SetToSeq(Ran)

SpecF == InitF /\ [][NextF]_<<vars, iorder, elog>>

SpecFL == SpecF /\ Fairness

Range(s) == {s[i] : i \in DOMAIN s}

First(s, x) == CHOOSE i \in DOMAIN s : s[i] = x /\ \A j \in 1..(i - 1) : s[j] # x

\* A first runs D's calls in the order that D admitted them.
PerSenderFIFO ==
    \A x, y \in Range(elog) :
        First(iorder, x) < First(iorder, y) => First(elog, x) < First(elog, y)

=============================================================================
