# The record writer

| | |
| --- | --- |
| **Feature** | `yoke-verify record` turns a run's own output into one record of what every declared case did, in a schema carrying its version, computing what no runner reports |
| **Planning item** | yoke-project/yoke#4 |

## yoke:record.01 — a run writes one record, and it carries every field of the schema

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree with one description and its tests, the results of a run over it, and the environment, tier and artifacts of that run as the run states them |
| **Action** | run `yoke-verify record` over that tree, with those results |
| **Expected** | it emits one JSON object holding `schema` as an integer, `repository`, `commit`, `level`, `tier`, `environment`, `started`, `finished`, `ran`, `state`, `blocks` and `cases` — and the cases in identifier order, so two runs of the same commit differ only where the runs differ |

## yoke:record.02 — a test's result reaches its case through the map

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §From a test's result to its case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree whose descriptions and tests correspond, and the runner's output naming the tests it ran and what each did |
| **Action** | run `yoke-verify record` over that tree, with that output |
| **Expected** | every entry answers to a case's identifier and never to a test's name, and a passing test becomes `pass` on the case its marker names — the scan that checked the correspondence is the one that produced the map, so a record cannot be joined by a map nobody verified |

## yoke:record.03 — a runner's skip is recorded as a failure

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record · testing/20 §What a pass is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the runner's output reporting one test as skipped, and the rest as passing |
| **Action** | run `yoke-verify record` over that tree, with that output |
| **Expected** | the skipped test's case is recorded `fail`, with the skip as its detail — nothing is skipped, and a skip recorded as anything else is a case that stopped being verified without anybody deciding it |

## yoke:record.04 — a case no result answers for is absent, and absent is a failure

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description declaring three cases at this level, and a run whose output answers for two of them |
| **Action** | run `yoke-verify record` over that tree, with that output |
| **Expected** | the third is recorded `absent`, the record's `state` is `failed` and `blocks` is true — a case whose test did not run is a rule nobody showed to hold, which is the same standing as one shown not to |

## yoke:record.05 — not applicable is computed from the description, never reported

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record · testing/20 §The environments |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description whose case declares **Not applicable in** a dimension and value the run fixed, and a run whose output says nothing about it |
| **Action** | run `yoke-verify record` over that tree, stating the environment |
| **Expected** | the case is recorded `not applicable` and not `absent`, it makes neither `state` nor `blocks` move, and the same case in an environment the declaration does not name is recorded `absent` — what a case does not apply to is a declaration, and a runner is never asked about it |

## yoke:record.06 — the label is recorded as the description declared it

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record · testing/20 §Blocking and not blocking |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a description whose cases carry `blocking` and `not blocking — <owner>/<repository>#<number>` |
| **Action** | run `yoke-verify record` over that tree, with a run's output |
| **Expected** | each entry carries its label as the description gives it, and a not-blocking entry carries the item it cites — so a record read a year later still says what its failures stopped on the day it was written |

## yoke:record.07 — blocks is what failed, and not how much

| Field | Value |
| --- | --- |
| **Cites** | testing/20 §Blocking and not blocking · testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three runs of one tree: one where only a not-blocking case fails, one where a blocking case fails, and one where a blocking case is absent |
| **Action** | run `yoke-verify record` for each |
| **Expected** | `blocks` is false in the first and true in the other two, while `state` is `failed` in all three — the label changes what a failure stops and never whether it failed |

## yoke:record.08 — a level whose predecessor failed is recorded as not reached

| Field | Value |
| --- | --- |
| **Cites** | testing/20 §The order between levels |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree whose level did not run, because the level before it failed |
| **Action** | run `yoke-verify record` for that level, saying it was not reached |
| **Expected** | `state` is `not reached`, every case is `absent` and no case carries that state — it is a state of the level, and a record that called it a pass would hide a level nobody ran |

## yoke:record.09 — a result for a case no description declares refuses the record

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record · testing/30 §Joining a case to its test |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a run's output naming a test that no marker joins to any case |
| **Action** | run `yoke-verify record` over that tree, with that output |
| **Expected** | it exits non-zero naming the test and writes no record — a result nobody can attribute is evidence of a map that has stopped matching the tree, and a record written around it would be evidence of nothing |

## yoke:record.10 — the two forms a run of this project emits are both read

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §From a test's result to its case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | one tree, and two runs over it: a Go runner's own machine-readable output, and the lines a check writes for itself |
| **Action** | run `yoke-verify record` over each |
| **Expected** | both produce the same entries for the cases they answer for — a check has no runner to translate, so the tool reads what a check already prints rather than asking twelve repositories to print something new |

## yoke:record.11 — the commit is read from a worktree

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository with one commit, and a worktree of it on a branch of its own, where `.git` is a file naming the worktree's directory |
| **Action** | run `yoke-verify record` over the worktree, giving no commit |
| **Expected** | the record's `commit` is the commit the worktree's branch names |

## yoke:record.12 — the commit is read from packed references

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository with one commit, its references packed, so the branch has no file of its own and exists only as a line of `packed-refs` |
| **Action** | run `yoke-verify record` over it, giving no commit |
| **Expected** | the record's `commit` is the commit the branch names |

## yoke:record.13 — a record that can name no commit is refused

| Field | Value |
| --- | --- |
| **Cites** | testing/40 §A record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree that is not a repository |
| **Action** | run `yoke-verify record` over it, giving no commit |
| **Expected** | no record is written, and the refusal says the commit could not be read — a record that names no commit cannot be joined to anything |
