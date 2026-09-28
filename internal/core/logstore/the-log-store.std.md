# The log store

| | |
| --- | --- |
| **Feature** | the store of evidence, `logs.db` in the instance's state directory: entries that belong to a unit and one of its lives, or to no unit at all; numbered as they are stored, which is the order it keeps; appended in batches; the persistent counter that numbers a unit's lives; and what reaches it — a unit's output line by line, the Core's events as their durable counterparts, a unit's reports at the grade it declared |
| **Planning item** | yoke-project/yoke#89 |

## yoke:the-log-store.01 — an entry belongs to a unit and one of its lives, to a unit alone, or to no unit

| Field | Value |
| --- | --- |
| **Cites** | specs/30.35 · specs/30.36 · specs/30.42 · arch/40-state/03 §What an entry belongs to |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty log store |
| **Action** | append an entry of `acquire`'s second life, one of `acquire` with no life, one of no unit, and one with a life and no unit |
| **Expected** | the first three are stored and read back with their unit and life as given — the two absences kept apart; the fourth is refused, and nothing of it is stored |

## yoke:the-log-store.02 — entries are numbered as they are stored, and that is the order the store keeps

| Field | Value |
| --- | --- |
| **Cites** | specs/30.37 · arch/40-state/03 §Insertion order is the total order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty log store |
| **Action** | append three entries whose own times run backwards: the first an hour after the second, the second an hour after the third |
| **Expected** | they are numbered 1, 2 and 3 in the order appended, and read back in that order, each with the time it said about itself |

## yoke:the-log-store.03 — entries are appended in batches, and a quiet deployment's last line is durable within a tenth of a second

| Field | Value |
| --- | --- |
| **Cites** | specs/30.34 · arch/40-state/03 §Insertion order is the total order · arch/40-state/03 §The figures this document sets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty log store, and a second connection reading the same file |
| **Action** | append 64 entries at once; then one more, and nothing after it |
| **Expected** | the 64 are readable from the other connection before a tenth of a second has passed, in the order appended; the last is readable within a quarter of a second |

## yoke:the-log-store.04 — a unit's lives are counted once per launch, and the count survives the Core

| Field | Value |
| --- | --- |
| **Cites** | specs/30.39 · specs/30.40 · specs/30.41 · arch/40-state/03 §The counter |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty log store |
| **Action** | count two launches of `acquire`, and one of `archive`; close the store and open it again; count one more of `acquire`; then twenty of `probe` at once |
| **Expected** | `acquire` is 1 and 2, `archive` 1, and after reopening `acquire` is 3; the twenty launches of `probe` are twenty different numbers, 1 to 20 |

## yoke:the-log-store.05 — the Core's events reach the store as their durable counterparts, and a unit's report at the grade it declared

| Field | Value |
| --- | --- |
| **Cites** | specs/31.21 · specs/30.17 · specs/31.18 · arch/40-state/03 §Where entries come from · arch/45-events/06 §Durable counterparts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus, and a log store keeping the counterpart of each event where it is published |
| **Action** | publish a state change of `acquire`'s first life into `Failed`, caused by an earlier event; the instance becoming ready; and `acquire`'s report of `calibration.drift` at 97 |
| **Expected** | three entries: the state change with source `core`, attributed to `acquire` and its life, its type, subject, actor, severity 50, cause and detail; the readiness with source `core`, no unit, and the instance as its subject; the report with source `reported`, severity 97 and the unit's detail |

## yoke:the-log-store.06 — a write that fails once the store is open is reported, and is not fatal

| Field | Value |
| --- | --- |
| **Cites** | arch/40-state/03 §Insertion order is the total order · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open log store, whose connection to its file then fails |
| **Action** | append an entry |
| **Expected** | appending returns; the failure is reported to whoever opened the store, naming it; the store can still be closed |

## yoke:the-log-store.07 — through the Core, a unit's output and its life are recorded, and its next life is counted after the Core restarts

| Field | Value |
| --- | --- |
| **Cites** | specs/30.35 · specs/30.40 · specs/31.21 · arch/40-state/03 §What is in it |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition running one oneshot unit that prints a line to its standard output and one to its standard error, and exits zero |
| **Action** | start the Core, stop it once the unit completed, and start it again |
| **Expected** | `logs.db` in the state directory holds the two lines with sources `stdout` and `stderr`, attributed to the unit's first life; the unit's state changes of that life, with source `core`; and, after the second start, lines attributed to its second life |
