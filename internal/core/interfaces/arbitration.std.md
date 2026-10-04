# Arbitration

| | |
| --- | --- |
| **Feature** | arbitration between live channels: computed on every event that can change whether a channel holds — an attachment, a detachment, a subscription gone stale or confirmed again, a reclaim — and never on a clock; a channel that prevails and holds suspends each channel it prevails over in that channel's declared grade, and a change in the result, and nothing else, is published; a suspended channel keeps its connection, and its grade withdraws what it withdraws, refused with `channel.suspended` naming the grade and the channel that prevails; a channel bound as a local socket may always reclaim control, and no grade withdraws that; and a suspension is a state of the channel its record carries |
| **Planning item** | yoke-project/yoke#144 |

## yoke:arbitration.01 — a channel that prevails and holds suspends the channels it prevails over, and a change is all that is published

| Field | Value |
| --- | --- |
| **Cites** | specs/70.24 · specs/70.27 · specs/70.31 · arch/70-interface-surface/06 §What the declaration says, and what holding means · arch/70-interface-surface/06 §When it is computed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two channels, `bench` declared to prevail over `remote`, and a third channel no rule names |
| **Action** | attach to `remote`, then to `bench`, then to the third; detach from `bench` |
| **Expected** | `remote` is suspended when `bench` attaches — `channel.suspended` with the reason, the channel that prevailed and what `remote` retains — and resumed when `bench` detaches, with `channel.resumed`; attaching to the third publishes neither |

## yoke:arbitration.02 — a channel whose subscription goes stale stops holding

| Field | Value |
| --- | --- |
| **Cites** | specs/70.31 · specs/70.32 · specs/70.35 · arch/70-interface-surface/06 §How a suspension ends, in two halves |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `bench` prevailing over `remote`, both attached, with a confirmation interval of 100 ms and a tolerance of three |
| **Action** | confirm on `remote` throughout, and never on `bench`; then confirm on `bench` again |
| **Expected** | `remote` is resumed when `bench`'s subscription goes stale, and suspended again when `bench` confirms |

## yoke:arbitration.03 — a grade withdraws what it withdraws, and a suspended channel keeps its connection

| Field | Value |
| --- | --- |
| **Cites** | specs/70.26 · specs/70.28 · specs/70.29 · specs/70.30 · arch/70-interface-surface/06 §What each grade withdraws · arch/70-interface-surface/05 §What a suspended channel keeps |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `bench` prevailing over `remote`, declared read-only by saying nothing, and over `dark`, declared dark; all attached, `bench` last |
| **Action** | on `remote`, `command` and `query` a unit and `read` it; on `dark`, `query` it and read its channel; then publish an event about a unit, and one about each suspended channel |
| **Expected** | on `remote` the command is refused `channel.suspended` naming the grade `read-only` and `bench`, and the question and the read are not; on `dark` the question is refused naming `dark` and `bench`, and the read of its own channel answered; `remote`'s standing subscription carries the unit's event, and `dark`'s carries only the event about itself; neither connection is closed |

## yoke:arbitration.04 — a channel bound as a local socket may always reclaim, and nothing else may

| Field | Value |
| --- | --- |
| **Cites** | specs/70.36 · specs/70.37 · specs/70.38 · specs/70.39 · specs/70.40 · arch/70-interface-surface/06 §A local channel may always reclaim control |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `bench`, on loopback, prevailing over `panel`, on a local socket and declared dark; both attached |
| **Action** | `reclaim` on `panel`; `reclaim` on `bench`; detach from `panel` |
| **Expected** | the reclaim on `panel` is answered with its record no longer suspended and that something changed; `panel` is resumed and `bench` suspended in its own grade, with the reason `reclaimed`; the reclaim on `bench`, a loopback channel, is refused `channel.not_local`; when `panel` detaches the declared rule applies again, and `bench` is resumed |

## yoke:arbitration.05 — a suspension is a state of the channel, which its record carries

| Field | Value |
| --- | --- |
| **Cites** | specs/70.27 · arch/70-interface-surface/06 §How a displaced client learns · arch/70-interface-surface/05 §What a channel observes, and what it may address |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `bench` prevailing over `remote`, both attached |
| **Action** | read `remote`'s channel on `remote`; attach to `remote`'s twin a second client, where `remote` takes several |
| **Expected** | the record says suspended, its grade, the channel that prevails and the reason; the second client's opening picture says the same |

## yoke:arbitration.06 — through the Core, a channel that prevails suspends another, and its command is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/70.24 · specs/70.26 · arch/70-interface-surface/06 §What each grade withdraws |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring two local channels, `bench` prevailing over `panel` |
| **Action** | attach to `panel`, then to `bench`; command a unit on `panel` |
| **Expected** | the Core's output records `channel.suspended` about `panel`; the command is refused `channel.suspended` naming `bench` |
