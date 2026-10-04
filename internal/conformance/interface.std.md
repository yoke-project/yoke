<!-- Rendered by yoke-conformance from its cases: change the cases, not this file. -->
# The interface contract, at L2

| | |
| --- | --- |
| **Feature** | the interface contract, as the conformance suite measures it against a real Core through a family's harness |
| **Planning item** | yoke-project/yoke#148 |

## yoke:interface.01 — attaching is given the opening: the version, the standing subscription and the channel's own record

| Field | Value |
| --- | --- |
| **Cites** | specs/70.9 · specs/90.8 · arch/70-interface-surface/03 §The opening picture · arch/70-interface-surface/08 §How this contract states its version |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core started by the suite with the channels `panel` and `bench`, local and of one client each, `bench` prevailing over `panel`; and the harness launched by the suite with the instance's root in `CONFORMANCE_INSTANCE` |
| **Action** | `attach` to `panel` |
| **Expected** | the version 1, a standing subscription, and a picture holding `panel`'s record, attached |

## yoke:interface.02 — a channel of one client refuses another, and keeps the first

| Field | Value |
| --- | --- |
| **Cites** | specs/70.21 · arch/70-interface-surface/03 §How many clients |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel` |
| **Action** | `attach` to `panel` again; then `read` of the kind `channel` |
| **Expected** | a refusal `channel.in_use`; then the read answered on the first attachment |

## yoke:interface.03 — a refusal travels as its code, with what it names

| Field | Value |
| --- | --- |
| **Cites** | specs/90.23 · arch/00-system/05 §How a refusal travels · arch/70-interface-surface/08 §This surface's error codes |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`, and a unit nobody declared |
| **Action** | `read` of the unit `nobody` |
| **Expected** | `subject.unknown` naming the kind `unit` and the identity `nobody` |

## yoke:interface.04 — a displaced channel learns as a subscriber, and what it may not do is refused with its grade

| Field | Value |
| --- | --- |
| **Cites** | specs/70.26 · specs/70.27 · specs/70.29 · arch/70-interface-surface/06 §What the declaration says, and what holding means |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`; then the suite attaches to `bench` |
| **Action** | nothing, until the standing subscription carries the suspension; then `command` of `calibrate` to the fixture |
| **Expected** | an event `channel.suspended` about `panel`; then a refusal `channel.suspended`, in the grade `read-only`, in favour of `bench` |

## yoke:interface.05 — a channel on a local socket reclaims, and its suspension ends

| Field | Value |
| --- | --- |
| **Cites** | specs/70.35 · specs/70.36 · specs/70.37 · arch/70-interface-surface/06 §How a suspension ends, in two halves |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`, suspended in favour of `bench` |
| **Action** | `reclaim` |
| **Expected** | that it changed something, and `panel`'s record no longer suspended |

## yoke:interface.06 — a confirmation is answered

| Field | Value |
| --- | --- |
| **Cites** | specs/70.32 · specs/70.33 · arch/70-interface-surface/03 §A connection that looks open proves nothing |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`, a channel named in an arbitration rule |
| **Action** | `confirm` of the standing subscription |
| **Expected** | an answer, and no refusal |

## yoke:interface.07 — a subscription opens with a snapshot of what the channel sees

| Field | Value |
| --- | --- |
| **Cites** | specs/70.7 · specs/90.32 · arch/70-interface-surface/05 §What a subscription promises |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel` |
| **Action** | `subscribe` to the subject kind `channel` |
| **Expected** | a snapshot holding the records of `panel` and of `bench` |
