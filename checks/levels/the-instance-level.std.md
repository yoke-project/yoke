# The instance level

| | |
| --- | --- |
| **Feature** | this repository's programs are executed on a real host through the `test` verb, and each run hands over one record per level it performed — L1 and L3 — with L3 not reached when L1 failed |
| **Planning item** | yoke-project/yoke#49 |

## yoke:the-instance-level.01 — a run hands over one record per level, each as an artifact of its own

| Field | Value |
| --- | --- |
| **Cites** | testing/20 §The levels · testing/40 §A record · prj_structure/95 §Continuous integration |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | read the workflow that verifies a change |
| **Expected** | after the verbs, whatever they decided, it writes the record of L1 and the record of L3 with the script that assembles a record, and uploads each as an artifact named `record-L1` and `record-L3` — a level's evidence is handed over apart from another's, so the collection reads it as that level's |

## yoke:the-instance-level.02 — L3 is not reached when L1 blocks, and counts when it does not

| Field | Value |
| --- | --- |
| **Cites** | testing/20 §The order between levels · testing/40 §A record |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a run's results, and a record of L1 that blocks; then the same results and a record of L1 that does not |
| **Action** | assemble the record of L3 after each |
| **Expected** | the first is `not reached`; the second is not — until the conformance suite runs, L1 is what L3 follows, and a level whose predecessor failed was never run |

## yoke:the-instance-level.03 — a Core killed with no cleanup leaves its instance to the next Core

| Field | Value |
| --- | --- |
| **Cites** | specs/25.12 · specs/25.13 · arch/30-core/02 §The claim |
| **Level** | L3 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, a `core.yaml` whose directories are the check's own, and a Core serving it |
| **Action** | start a second Core; kill the first with `SIGKILL`; start a third |
| **Expected** | the second exits non-zero naming the first's process, the third serves the instance, and the lock file the first left is still there — the kernel released the claim, and a crash needs nobody to clean up before the instance can run again |
