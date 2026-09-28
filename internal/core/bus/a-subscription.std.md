# What a subscription promises

| | |
| --- | --- |
| **Feature** | a subscription that opens with a snapshot — the current value of every level its filter selects, taken at a sequence the stream then resumes after — that carries only what its filter selects, that is told of an overflow and handed a fresh snapshot at a new sequence rather than left to diverge, and whose join between the picture and the stream is exact |
| **Planning item** | yoke-project/yoke#90 |

## yoke:a-subscription.01 — a subscription opens with a snapshot of the levels its filter selects

| Field | Value |
| --- | --- |
| **Cites** | specs/31.27 · specs/31.21 · arch/45-events/04 §1 — The snapshot · arch/45-events/03 §The rule that joins them to the stores |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus on which `acquire` changed into `Running`, `archive` into `Failed`, `acquire` reported an occurrence and the instance became ready |
| **Action** | subscribe with a filter selecting the subject kind `unit`; then publish a state change of `acquire` |
| **Expected** | the snapshot holds the two state changes and nothing else — an edge has nothing to be current about, and the instance is outside the filter — and is taken at the last sequence published; the first event delivered is numbered one after it |

## yoke:a-subscription.02 — the snapshot holds each subject's current value, and nothing it replaced

| Field | Value |
| --- | --- |
| **Cites** | specs/31.27 · arch/45-events/04 §1 — The snapshot |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus on which `acquire`'s first life changed into `Starting`, `Admitted` and `Running`, and its second life into `Starting` |
| **Action** | subscribe with no filter |
| **Expected** | the snapshot holds one state change for `acquire`: its second life's, into `Starting` |

## yoke:a-subscription.03 — a filter narrows the stream as it narrows the snapshot

| Field | Value |
| --- | --- |
| **Cites** | specs/31.39 · specs/31.28 · arch/45-events/04 §2 — Bounded delivery |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a subscription selecting severity 50 or above |
| **Action** | publish a unit changing into `Running`, one changing into `Failed`, and an occurrence at 90 |
| **Expected** | it is told the failure and the occurrence, in that order, and nothing else |

## yoke:a-subscription.04 — an overflow is followed by a fresh snapshot, and the stream resumes after it

| Field | Value |
| --- | --- |
| **Cites** | specs/31.29 · arch/45-events/04 §3 — Overflow |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a subscription that does not read, and 300 state changes of `acquire` published to it, the last into `Failed` |
| **Action** | read it until it announces the overflow; publish one more change; read it |
| **Expected** | after what its queue held it is told of the overflow with a snapshot holding `acquire`'s change into `Failed`, taken at the last sequence published; the next event it is told is the one published after, numbered one past that |

## yoke:a-subscription.05 — the join between the picture and the stream is exact

| Field | Value |
| --- | --- |
| **Cites** | specs/31.27 · specs/31.30 · arch/45-events/04 §1 — The snapshot · arch/45-events/04 §4 — Ordering |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | ten units whose states change continuously on the bus |
| **Action** | subscribe while the changes are being published, read what follows, and stop publishing |
| **Expected** | every event delivered is numbered after the snapshot, in increasing order; for every unit, the last value the subscriber holds — delivered, or else in the snapshot — is the last change published for it |
