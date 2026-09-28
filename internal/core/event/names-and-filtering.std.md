# Names, filtering, and the catalogue

| | |
| --- | --- |
| **Feature** | the grammar every type is named under and the subject kind it agrees with; a type existing once a producer does — the instance, a unit's life and its condition, its occurrences, and the documents discovery reads; the four independent axes a subscription selects on, combined by conjunction; an unknown type placed and ranked without being understood; and every type's durable counterpart in the log store |
| **Planning item** | yoke-project/yoke#88 |

## yoke:names-and-filtering.01 — every type follows the grammar, and agrees with its subject

| Field | Value |
| --- | --- |
| **Cites** | specs/31.36 · arch/45-events/05 §The grammar |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the types the Core declares |
| **Action** | read each name; then check a state change whose type names another subject kind than its subject's |
| **Expected** | every name is lowercase, two or three dot-separated segments, its first segment one of the six kinds; the state change is refused, naming the disagreement |

## yoke:names-and-filtering.02 — a type exists once a producer does, and every declared type has one

| Field | Value |
| --- | --- |
| **Cites** | specs/31.38 · specs/31.42 · arch/45-events/05 §A name is created when a producer exists · arch/45-events/06 §The set |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Core's source, tests excepted |
| **Action** | for each declared type, look for a call of the constructor that makes it outside the event package |
| **Expected** | every declared type is made somewhere else in the Core — the trunk, the supervisor, the Session or discovery — and none is declared ahead of its producer |

## yoke:names-and-filtering.03 — four independent axes, combined by conjunction

| Field | Value |
| --- | --- |
| **Cites** | specs/31.39 · specs/31.40 · arch/45-events/05 §Filtering has four independent axes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | events of `acquire` and `archive` changing state at 10 and 50, a stream of `acquire` activated, and `acquire` reporting `calibration.drift` and `calibration.offset` |
| **Action** | filter them by nothing; by the subject kind `unit`; by the subject `acquire`; by the floor 50; by the type `unit.state.changed`; by the type prefix `unit.stream`; by the type prefix `unit.st`; by the occurrence prefix `calibration`; by the subject `acquire` and the floor 50 |
| **Expected** | nothing selects all; the kind all of them; `acquire` its four; the floor both failures and whatever report is graded 50 or above; the type both state changes; the prefix `unit.stream` the activation and `unit.st` nothing, since a prefix reads segments; the occurrence both reports and nothing else; `acquire` at 50 or above its failure alone |

## yoke:names-and-filtering.04 — a type a consumer has never seen is placed and ranked without being understood

| Field | Value |
| --- | --- |
| **Cites** | specs/31.41 · arch/45-events/05 §An unknown type must be safely ignorable |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an event of a type nobody declared, `unit.lamp.changed`, about `acquire` at 70, with a detail that is not an object |
| **Action** | filter it by the subject `acquire` and the floor 50 |
| **Expected** | it is selected: its subject and its severity are enough, and its detail is never read |

## yoke:names-and-filtering.05 — every declared type's durable counterpart is an entry in the log store

| Field | Value |
| --- | --- |
| **Cites** | specs/31.21 · specs/31.44 · arch/45-events/06 §Durable counterparts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | one event of every declared type |
| **Action** | take each one's durable counterpart |
| **Expected** | each is a log entry carrying its type and subject; a unit's report has the source `reported`, everything else `core`; a unit's events are attributed to the unit and its life, and nothing else to any unit |

## yoke:names-and-filtering.06 — a unit's condition changes when its grade does, and not otherwise

| Field | Value |
| --- | --- |
| **Cites** | specs/50.66 · arch/50-plugin-surface/05 §What a health report carries · arch/45-events/06 §The set |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session of a unit that has never reported its health |
| **Action** | the unit reports 90 with a line; 90 with the same line; 40 with another |
| **Expected** | two `unit.condition.changed` are published: the first to 90 with no former grade, the second from 90 to 40, each with its line, graded as the unit graded it and with the unit as its actor |

## yoke:names-and-filtering.07 — through the Core, the documents discovery reads are published and recorded

| Field | Value |
| --- | --- |
| **Cites** | specs/31.48 · specs/20.42 · arch/45-events/06 §The set · arch/30-core/07 §What it reads |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a Plugin directory holding one Manifest that passes and one that does not parse, and a composition that passes |
| **Action** | start the Core |
| **Expected** | its output records `document.resolved` for the good Manifest, with its digest, and for the composition; and `document.rejected` for the other, naming the finding; the log store holds the three as entries about those documents |
