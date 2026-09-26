# The Core binary

| | |
| --- | --- |
| **Feature** | `yoke-core` as a program: it serves one instance at a time and says which process does, and a configuration that does not pass keeps it from starting |
| **Planning item** | yoke-project/yoke#7 |

## yoke:the-core-binary.01 — the Core serves one instance at a time, and says who does

| Field | Value |
| --- | --- |
| **Cites** | specs/25.10 · specs/25.13 · arch/30-core/01 §The service form · arch/30-core/02 §The claim |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, and a `core.yaml` named by `YOKE_CONFIG` whose `runtime_dir` and `state_dir` are in a directory of the test's own |
| **Action** | start it; start it a second time; stop the first with `SIGTERM`; start it a third time |
| **Expected** | the first creates the runtime directory and holds the claim; the second exits non-zero, saying the instance is already running as the first's process identifier, and removes nothing; the first exits zero; the third holds the claim |

## yoke:the-core-binary.02 — a configuration that does not pass prevents startup

| Field | Value |
| --- | --- |
| **Cites** | specs/20.5 · specs/20.7 · arch/30-core/01 §Configuration |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, and a `core.yaml` with an unknown key whose `runtime_dir` is in a directory of the test's own |
| **Action** | start it |
| **Expected** | it exits non-zero naming the key, before the runtime directory exists — validation is a gate, and nothing is created by a process that will not run |
