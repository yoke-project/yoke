# The cases the tool emits

| | |
| --- | --- |
| **Feature** | `yoke-verify` emits the cases it parsed, with their citations as written, so that resolving them stays where the corpora are |
| **Planning item** | yoke-project/yoke#38 |

## yoke:emitted-cases.01 — every live case is emitted with its identifier, its file and its eight fields

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §What checks a description · testing/30 §A case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree with two descriptions, in more than one directory, whose cases hold the form |
| **Action** | run `yoke-verify descriptions` over that tree, asking it to emit what it parsed |
| **Expected** | one JSON entry per live case, in the order the descriptions give them, each naming the case's identifier, the file it was found in, its title, and the eight fields with the values the description wrote — what the tool read, and nothing it inferred |

## yoke:emitted-cases.02 — a struck case is emitted as struck, and carries no fields

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The identifier of a case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description whose second case is struck |
| **Action** | run `yoke-verify descriptions` over that tree, asking it to emit what it parsed |
| **Expected** | the struck case appears, marked as struck and with no fields — a reader counting what is verified must be able to tell a number that is taken from a case that is live, and neither guess from the other |

## yoke:emitted-cases.03 — the citations come out as they were written

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §A case · testing/30 §What checks a description |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a case citing three rules of different kinds, separated as a description separates them |
| **Action** | run `yoke-verify descriptions` over that tree, asking it to emit what it parsed |
| **Expected** | the three citations come out as three, each the text the description wrote, in its order — unresolved, unrewritten and unsorted: what a citation means is known where the corpora are, and never here |

## yoke:emitted-cases.04 — nothing is emitted for a description that does not hold the form

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §What checks a description |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree with one description in the form and one whose case lacks a field |
| **Action** | run `yoke-verify descriptions` over that tree, asking it to emit what it parsed |
| **Expected** | it exits non-zero, reports the finding, and emits nothing at all — not even the description that held — so what a reader consumes is only ever descriptions their own repository has already accepted |
