# The description checker and the marker scan

| | |
| --- | --- |
| **Feature** | `yoke-verify` reads a repository's descriptions, refuses every one that does not hold the form, and emits the correspondence between a case and the test that performs it |
| **Planning item** | yoke-project/yoke#3 |

## yoke:descriptions-and-markers.01 — a description in the form is read, and its cases are reported in order

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The file · testing/30 §A case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree holding one `<feature>.std.md` with one `H1`, the two rows **Feature** and **Planning item**, and two cases whose eight fields appear in the stated order |
| **Action** | run `yoke-verify descriptions` over that tree |
| **Expected** | it exits zero and names the two identifiers in the order the file gives them, and reports no finding — a description that holds the form produces no output a reader has to dismiss |

## yoke:descriptions-and-markers.02 — a field missing, repeated, out of order or unknown is refused

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §A case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | four descriptions, each with one case: one lacking **Expected**, one carrying **Cites** twice, one with **Method** before **Level**, and one carrying a ninth field |
| **Action** | run `yoke-verify descriptions` over each |
| **Expected** | each exits non-zero with a finding naming the case's identifier and the field at fault — the order is checked and not only the set, because a parser that tolerated variation would let a description stop saying what it seems to say |

## yoke:descriptions-and-markers.03 — a value outside what a field allows is refused

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §A case · testing/20 §The levels · testing/20 §The environments |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | descriptions carrying, one each, **Level** `L2`, **Method** `verify`, **Not applicable in** with a dimension no environment declares, and **Label** `blocking — yoke-project/yoke#3` |
| **Action** | run `yoke-verify descriptions` over each |
| **Expected** | each exits non-zero naming the field and the value refused; `L2` is refused here in particular, since it exists only in the suite's own rendering and never in a description written by hand |

## yoke:descriptions-and-markers.04 — an identifier that does not match its file, or repeats, is refused

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The identifier of a case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | descriptions carrying, one each, an identifier whose feature is not the file's basename, one whose repository is not this repository, one numbered `.1`, and two cases sharing one number |
| **Action** | run `yoke-verify descriptions` over each |
| **Expected** | each exits non-zero naming the identifier at fault — a collision is refused in the repository, where it is cheap, rather than in the trace that aggregates twelve of them |

## yoke:descriptions-and-markers.05 — a not-blocking label cites an item

| Field | Value |
| --- | --- |
| **Cites** | testing/20 §Blocking and not blocking · testing/30 §What checks a description |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description whose case carries **Label** `not blocking`, with no item, and another carrying `not blocking — yoke-project/yoke#3` |
| **Action** | run `yoke-verify descriptions` over each |
| **Expected** | the first is refused naming the identifier, the second is accepted — a failure allowed to pass with nothing to point at is a failure nobody is going to come back for |

## yoke:descriptions-and-markers.06 — every case has exactly one marker, and every marker names exactly one case

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree with a description of three cases beside its tests, in three states: one case no marker names, one marker naming an identifier no description declares, and two tests carrying the same marker |
| **Action** | run `yoke-verify markers` over that tree |
| **Expected** | it exits non-zero with one finding per state, each naming the identifier and the file it was found in — the correspondence is exact in both directions, because a case with two tests has two results and no rule for combining them |

## yoke:descriptions-and-markers.07 — the scan emits the correspondence it checked

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §From a test's result to its case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree whose descriptions and tests correspond exactly, in more than one file |
| **Action** | run `yoke-verify markers` over that tree, capturing what it emits |
| **Expected** | it exits zero and emits every pair — the test, the file it is in, and the case's identifier — in a form the record writer reads back; one scan both checks the correspondence and produces the map, so the map cannot be right for the check and wrong for the report |

## yoke:descriptions-and-markers.08 — a marker in any of the project's languages is found

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree whose tests carry their markers in a Go, a Rust, a Python, a C, a C++ and a shell comment, one case each |
| **Action** | run `yoke-verify markers` over that tree |
| **Expected** | every case is matched to its test, in every language — the marker is a comment because a comment is the one construct all six have, and a scan that read only one of them would leave four families unable to report at all |

## yoke:descriptions-and-markers.09 — a struck case keeps its number and asks for nothing

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The identifier of a case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description whose second case is struck — its `H2` reading `~~<identifier> — <title>~~`, with no table — and tests marking only the cases that remain |
| **Action** | run `yoke-verify descriptions` and `yoke-verify markers` over that tree |
| **Expected** | both exit zero: the struck case is not demanded a table, not demanded a test, and does not appear in the map — it is kept to hold its number and for no other purpose |

## yoke:descriptions-and-markers.10 — a struck number cannot come back, and nothing may name it

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The identifier of a case · testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two trees: one whose description gives a live case the number a struck case above it holds, and one whose test carries a marker naming that struck case |
| **Action** | run `yoke-verify descriptions` over the first and `yoke-verify markers` over the second |
| **Expected** | each exits non-zero naming the identifier — a number is taken by having been used, and a record written against a reused identifier would answer to a case nobody could find |

## yoke:descriptions-and-markers.11 — a marker quoted in a fixture is not a marker

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree whose test carries its own marker as a comment line, and builds a fixture whose text holds another marker inside a string |
| **Action** | run `yoke-verify markers` over that tree |
| **Expected** | only the comment line is read, and the quoted one enters no map and raises no finding — a marker is the whole of a comment line, which is what lets a scan tell one from a fixture quoting one |
