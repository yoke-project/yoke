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
| **Cites** | specs/70.21 · specs/70.45 · arch/70-interface-surface/03 §How many clients |
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
| **Cites** | specs/31.45 · specs/70.3 · specs/70.26 · specs/70.27 · specs/70.29 · specs/70.45 · arch/70-interface-surface/06 §What the declaration says, and what holding means |
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
| **Cites** | specs/31.47 · specs/70.35 · specs/70.36 · specs/70.37 · arch/70-interface-surface/06 §How a suspension ends, in two halves |
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
| **Cites** | specs/31.35 · specs/70.32 · specs/70.33 · arch/70-interface-surface/03 §A connection that looks open proves nothing |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`, a channel named in an arbitration rule |
| **Action** | `confirm` of the standing subscription |
| **Expected** | an answer, and no refusal |

## yoke:interface.07 — a subscription opens with a snapshot of what the channel sees, and a channel sees itself alone

| Field | Value |
| --- | --- |
| **Cites** | specs/70.7 · specs/70.8 · specs/90.32 · arch/70-interface-surface/05 §What a subscription promises · arch/70-interface-surface/05 §Three subject kinds, and three it does not see |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel` |
| **Action** | `subscribe` to the subject kind `channel` |
| **Expected** | a snapshot holding `panel`'s record, and not `bench`'s |

## yoke:interface.08 — a library that reads the stream socket is delivered on one, and a frame above 1 MiB arrives whole

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §The three delivery paths |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`; the run's producer granted its stream, which flows on the shared ring with frames of 1572864 bytes |
| **Action** | `stream-subscribe` of the producer's stream, naming `stream_socket` among what the library reads |
| **Expected** | the delivery arriving on `stream_socket`; and a frame observed on it whose payload is 1572864 bytes, byte i being `a` + i mod 26 |

## yoke:interface.09 — a library that reads the shared object reads the ring itself, and the next activation's ring after a stop

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §The three delivery paths · arch/70-interface-surface/07 §What ends a delivery |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the producer's stream flowing, as case 8 left it |
| **Action** | `stream-subscribe` of it naming `shared_object`; then `stream-stop` and `stream-start` of it |
| **Expected** | the delivery arriving on `shared`; a frame read from the ring, whole; and after the stop and the start, a frame of sequence 1 on the same delivery, read from the next activation's ring |

## yoke:interface.10 — a question names its cause, and a declared channel refuses it

| Field | Value |
| --- | --- |
| **Cites** | specs/70.60 · arch/70-interface-surface/10 §A question asked on it |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`, a declared channel; the producer running, declaring the query `fixture.status` |
| **Action** | `query` of `fixture.status` of the producer, naming the cause `q-none` |
| **Expected** | a refusal `use.cause.unknown` naming the item `q-none`: the cause reached the Core, and only a need's channel takes one |

## yoke:interface.11 — a stream followed is subscribed again when its unit restarts

| Field | Value |
| --- | --- |
| **Cites** | specs/28.60 · arch/70-interface-surface/10 §When the unit bound restarts · arch/90-sdks/02 §What using a unit adds to every family |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the producer's stream flowing, as case 9 left it |
| **Action** | `stream-follow` of it; then, once a frame has arrived, the producer restarted on the administrative surface |
| **Expected** | a frame on the follow before the restart; and after it, on the same follow and a delivery other than the first, a frame of the next incarnation, whole |

## yoke:interface.12 — an event carries the line its producer wrote

| Field | Value |
| --- | --- |
| **Cites** | specs/31.3 · arch/45-events/01 §The nine fields · arch/90-sdks/02 §What a change to a contract costs |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness attached to `panel`; the producer, which reports how well it is with the line `producing` |
| **Action** | `subscribe` to the producer's `unit.condition.changed`; then the producer restarted on the administrative surface |
| **Expected** | an event `unit.condition.changed` about the producer, carrying the line `producing` |
