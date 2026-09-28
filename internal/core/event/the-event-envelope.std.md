# The event envelope, severity, level and edge

| | |
| --- | --- |
| **Feature** | what an event is before any bus carries it: eight fields, a subject that is a kind and an identity — a unit's with its incarnation — an actor the Core establishes, the Core's clock taken at the conclusion, a severity from 0 to 99 that the Core grades on its own conclusions and carries unchanged where a unit declared it, and a class — level or edge — declared with every type |
| **Planning item** | yoke-project/yoke#87 |

## yoke:the-event-envelope.01 — an event carries eight fields, and nothing that does not fit them

| Field | Value |
| --- | --- |
| **Cites** | specs/31.3 · specs/31.4 · specs/31.9 · specs/31.11 · arch/45-events/01 §The eight fields |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a well-formed `unit.state.changed` event |
| **Action** | check it; then check it with no type, with a subject of a seventh kind, with an empty subject identity, with an actor of a fifth class, at severity 100, carrying an occurrence; and a `unit.occurrence.reported` with none |
| **Expected** | the first passes and every other is refused, naming the field |

## yoke:the-event-envelope.02 — the subject is a kind and an identity, and a unit's carries its incarnation

| Field | Value |
| --- | --- |
| **Cites** | specs/31.4 · specs/31.5 · arch/45-events/01 §The subject is a kind and an identity |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit and a channel that share the name `station` |
| **Action** | conclude a state change of the unit's third incarnation, and build the channel's subject |
| **Expected** | the unit's subject is the kind `unit`, the identity `station` and the incarnation 3; the channel's is the kind `channel` and the same identity with no incarnation; the two are not equal |

## yoke:the-event-envelope.03 — the clock is the Core's, taken at the conclusion

| Field | Value |
| --- | --- |
| **Cites** | specs/31.8 · arch/45-events/01 §The clock is the Core's |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit's report of an occurrence, sent with a timestamp an hour in the past |
| **Action** | the Core turns it into an event |
| **Expected** | the event's time is the moment the Core concluded it, and nothing of the sender's timestamp |

## yoke:the-event-envelope.04 — the Core grades its own conclusions

| Field | Value |
| --- | --- |
| **Cites** | specs/31.13 · specs/31.14 · specs/31.15 · arch/45-events/02 §Two sources, and `actor` says which · arch/45-events/06 §The set |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | none |
| **Action** | conclude a unit's state changing into `Running`, `Failed`, `Refused` and `Stopped`, and the instance becoming ready |
| **Expected** | one type carries every state change, `from` and `to` in its detail, at 10, 50, 30 and 10; the instance is ready at 10; the actor of each is `core` |

## yoke:the-event-envelope.05 — a grade a unit declared is carried unchanged

| Field | Value |
| --- | --- |
| **Cites** | specs/31.14 · specs/31.15 · specs/31.18 · specs/31.40 · specs/31.49 · arch/45-events/02 §Nobody restates a declared value |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit's reports: an occurrence `calibration.drift` at 97 with a detail of bytes that are not text, the same at 0, and a condition graded 42 with a line |
| **Action** | the Core turns each into an event |
| **Expected** | the occurrences are `unit.occurrence.reported` at 97 and 0, the class in the `occurrence` field and not in the type, the detail the same bytes; the condition is `unit.condition.changed` at 42 with the line; the actor of each is `unit` |

## yoke:the-event-envelope.06 — every type is declared with its class

| Field | Value |
| --- | --- |
| **Cites** | specs/31.20 · specs/31.22 · arch/45-events/03 §The class is declared with the type |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | none |
| **Action** | ask the class of `unit.state.changed`, `unit.condition.changed`, `instance.ready`, `instance.stopping`, `document.resolved`, `unit.occurrence.reported`, `document.rejected` and `unit.lamp.changed`; check an event of the last |
| **Expected** | the first five are levels and the next two edges; the last has no class, and an event of it is refused |
