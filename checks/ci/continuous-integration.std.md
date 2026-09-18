# Continuous integration

| | |
| --- | --- |
| **Feature** | every proposed change to this repository runs its own verbs from a clean checkout, and what runs is decided by what moved |
| **Planning item** | yoke-project/yoke#23 |

## yoke:continuous-integration.01 — every proposed change runs the verbs, and nothing else does work

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §Continuous integration |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout, and the repository's workflow |
| **Action** | read the workflow's triggers and every command it runs |
| **Expected** | it runs on every proposed change; `build`, `test`, `lint` and `fmt` are each run as `just <verb>`; and every other command is one of the repository's own scripts under `ci/` — the obligation lives in the repository, and the platform only calls it |

## yoke:continuous-integration.02 — a change to no program's inputs selects no program job

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §Continuous integration |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the path filter, `ci/select-jobs.sh`, and a list of changed paths naming only a document outside the checks |
| **Action** | give the list to the filter |
| **Expected** | it selects no job — a document does not build a Core — and its answer is a value a test reads, not a behaviour only the platform observes |

## yoke:continuous-integration.03 — the suite and the version comparison are never skipped when the definitions move

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §Continuous integration · prj_structure/40 A6 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same filter, and a list of changed paths naming one file under the protocol definitions |
| **Action** | give the list to the filter |
| **Expected** | it selects **both** the conformance suite and the comparison of the two version statements, besides the verbs — a filter that can skip the check for the thing that changed is a hole and not a filter |

## yoke:continuous-integration.04 — what a run needs is declared, not taken from the runner

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/40 E1 |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout, and the repository's workflow |
| **Action** | read how each job obtains its machine and its tools |
| **Expected** | every job names a pinned runner image and never a moving one, and the runner and the language toolchain are installed at a stated version by the workflow itself — nothing a check needs is inherited from whatever an image happened to carry |
