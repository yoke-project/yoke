# A record per level, from one run

| | |
| --- | --- |
| **Feature** | one run may perform several levels, and the record of each takes the results of the cases declared at it |
| **Planning item** | yoke-project/yoke#49 |

## yoke:record-per-level.01 — a result for another level's case is left to that level's record

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §From a test's result to its case · testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description with one case at L1 and one at L3, the test performing each, and one run's output in which both passed |
| **Action** | write the record of L1, then the record of L3, from that one output |
| **Expected** | each record holds its own level's case, passed, and nothing of the other's; neither is refused — the level of a case is declared in its description, and a result answering a case at another level is evidence for that level's record, never an orphan |

## yoke:record-per-level.02 — a result answering no case at all still refuses the record

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §From a test's result to its case · testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same description, and an output that also names a test no marker joins to any case |
| **Action** | write the record of L3 |
| **Expected** | it exits non-zero naming the test, and writes no record — a result that belongs to no level is still an orphan |
