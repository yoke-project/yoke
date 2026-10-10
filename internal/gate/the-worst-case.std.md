# The worst case

| | |
| --- | --- |
| **Feature** | what a deployment's log store may occupy, computed by the gate before anything runs: the sum, over one group per declared unit and the Core's own, of each group's effective bytes limit; unbounded where any group's limit is zero, said as unbounded and never as a number, and never refused |
| **Planning item** | yoke-project/yoke#179 |

## yoke:the-worst-case.01 — the worst case is the sum of each group's effective bytes limit, one group per unit and the Core's

| Field | Value |
| --- | --- |
| **Cites** | specs/32.4 · specs/32.12 · arch/40-state/04 §The worst case, which is computed and not enforced |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition whose policy keeps ten megabytes, with a unit `a` whose own policy keeps one and a unit `b` with none |
| **Action** | check it, and ask the deployment its worst case |
| **Expected** | twenty-one megabytes, bounded: one for `a`, ten for `b`, ten for the Core's group |

## yoke:the-worst-case.02 — a group with no size limit makes the worst case unbounded, which is not refused

| Field | Value |
| --- | --- |
| **Cites** | specs/32.12 · specs/32.18 · arch/40-state/04 §The worst case, which is computed and not enforced |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composition of case 01, with `b` writing `bytes: 0` |
| **Action** | check it, and ask the deployment its worst case |
| **Expected** | the deployment passes with nothing refused, and its worst case is unbounded, with no figure beside it |
