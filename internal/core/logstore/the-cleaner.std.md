# The cleaner

| | |
| --- | --- |
| **Feature** | the only place Yoke deletes something nobody asked it to delete: per group — one per unit, and `/core` for what belongs to no unit — three independent limits applied in the order age, bytes, count, each seeing what the last left, a limit of zero constraining nothing; severity never read; a cycle every interval, the first one interval after startup plus a stable offset derived from the instance's name; `VACUUM` once enough has been deleted since the last |
| **Planning item** | yoke-project/yoke#177 |

## yoke:the-cleaner.01 — three limits, applied in the order age, bytes, count, each seeing what the last left

| Field | Value |
| --- | --- |
| **Cites** | specs/32.1 · specs/32.2 · specs/32.3 · arch/40-state/04 §The three limits |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a store whose group `acquire` holds twelve entries, the four oldest dated eight days ago and the rest today, each of a ten-byte message and the last two with a ten-byte detail |
| **Action** | clean the group with an age of seven days, then with eighty bytes, then with four entries |
| **Expected** | the age pass removes the four old entries; the bytes pass, measuring message and detail, removes the oldest of the eight until what is left measures eighty bytes or less; the count pass removes the oldest until four are left; what remains is the four newest |

## yoke:the-cleaner.02 — a limit of zero constrains nothing, and the defaults are the three figures

| Field | Value |
| --- | --- |
| **Cites** | specs/32.1 · arch/40-state/04 §The three limits |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same store |
| **Action** | clean the group with all three limits at zero; read the defaults |
| **Expected** | nothing is removed; the defaults are seven days, fifty megabytes and one hundred thousand entries |

## yoke:the-cleaner.03 — the group is the unit, and what belongs to no unit is the group `/core`

| Field | Value |
| --- | --- |
| **Cites** | specs/28.1 · specs/28.54 · specs/32.4 · specs/32.5 · specs/32.6 · specs/32.7 · arch/40-state/04 §The group is the unit |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a store holding a hundred entries of a noisy unit, five of a quiet one, three of the same quiet unit with no incarnation, and four belonging to no unit |
| **Action** | list the groups; run a cycle with a limit of ten entries for every group |
| **Expected** | the groups are the two units and `/core`; the noisy unit keeps its ten newest; the quiet unit keeps all eight, its entries without an incarnation counted in its group; `/core` keeps its four |

## yoke:the-cleaner.04 — severity is not read

| Field | Value |
| --- | --- |
| **Cites** | specs/32.33 · arch/40-state/04 §What retention does not read |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a group of six entries, the three oldest at severity 90 and the three newest at 10 |
| **Action** | clean it with a limit of three entries |
| **Expected** | the three oldest are removed, the severity notwithstanding |

## yoke:the-cleaner.05 — a cycle every interval, the first one interval after startup plus the instance's offset

| Field | Value |
| --- | --- |
| **Cites** | specs/32.21 · specs/32.23 · arch/40-state/04 §When the cleaner runs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the instance names `yoke`, `bench` and `bench-2`, an interval of one hour; a cleaner on a clock the test advances |
| **Action** | compute each name's offset twice; start a cleaner for `bench` and advance its clock |
| **Expected** | each offset is the same twice, lies in `[0, 1h)`, and the three differ; the first cycle runs one interval plus `bench`'s offset after the start and not before, and each next one an interval after the last |

## yoke:the-cleaner.06 — `VACUUM` once ten thousand deletions have accumulated since the last

| Field | Value |
| --- | --- |
| **Cites** | specs/32.24 · arch/40-state/04 §When the cleaner runs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a store that counts its vacuums, a group of six thousand entries refilled between cycles, and a limit of one entry |
| **Action** | run two cycles |
| **Expected** | the first cycle, having deleted fewer than ten thousand, does not vacuum; the second, bringing the count past it, vacuums once, and the count starts again |

## yoke:the-cleaner.07 — the Core schedules its cleaner one interval after startup, plus its offset

| Field | Value |
| --- | --- |
| **Cites** | specs/32.21 · specs/32.23 · arch/40-state/04 §When the cleaner runs |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a service-form instance with no unit |
| **Action** | start the Core |
| **Expected** | before it is ready it says when the cleaner's first cycle runs: one hour plus the offset of its instance's name after it started |
