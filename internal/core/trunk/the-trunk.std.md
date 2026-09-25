# The trunk, and the socket tree

| | |
| --- | --- |
| **Feature** | the eleven steps from `exec` to serving, the fatal boundary as a rule, readiness, stopping and the cleanup the next start owns; and the socket tree's root, its modes, a channel's socket and its ceiling |
| **Planning item** | yoke-project/yoke#8 |

## yoke:the-trunk.01 — the eleven steps run in their order

| Field | Value |
| --- | --- |
| **Cites** | specs/20.10 · specs/25.14 · arch/30-core/03 §The eleven steps |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk's steps |
| **Action** | run them, recording each as it starts |
| **Expected** | they are identity and paths, parameters, logging, the claim, debris, stores, inherited runtime facts, declarations, channels, ready, units — and each starts only when the one before it has finished, because each establishes what the next depends on |

## yoke:the-trunk.02 — a failure before readiness is fatal, wherever the step sits

| Field | Value |
| --- | --- |
| **Cites** | specs/20.12 · specs/25.18 · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk with its stores step failing; then the trunk with a step inserted just before readiness, failing |
| **Action** | run each |
| **Expected** | each stops at the failing step with an error naming it, and no later step starts — the boundary is readiness and not an ordinal, so a step inserted before it is fatal without anybody restating a number |

## yoke:the-trunk.03 — a failure after readiness is reported, and the rest goes on

| Field | Value |
| --- | --- |
| **Cites** | specs/20.12 · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk with its units step failing |
| **Action** | run it |
| **Expected** | the run completes, and the failure is written to the process logger naming the step — once the instance is observable there is somewhere to say it, and exiting would take a working instance away |

## yoke:the-trunk.04 — failing to ingest declarations is fatal in the application form alone

| Field | Value |
| --- | --- |
| **Cites** | specs/25.19 · specs/20.50 · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk with its declarations step failing, in the service form and in the application form |
| **Action** | run each |
| **Expected** | the service form reports it and reaches readiness; the application form stops — what a failure leaves is a deployment in one form and nothing in the other |

## yoke:the-trunk.05 — a channel that cannot be bound is fatal

| Field | Value |
| --- | --- |
| **Cites** | specs/25.20 · specs/25.36 · arch/30-core/04 §The two ceilings |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk with a channel whose path is 108 characters |
| **Action** | run it |
| **Expected** | it stops at the channels step, naming the path and its length, and readiness is never reached — a Core that cannot be reached where it was declared is worse than no Core |

## yoke:the-trunk.06 — debris is everything under the root but the claim

| Field | Value |
| --- | --- |
| **Cites** | specs/25.45 · specs/25.46 · arch/30-core/03 §Cleanup belongs to the next start |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root holding a previous incarnation's sockets, a unit's socket, a subdirectory with a stream's socket, and the lock file; the claim free |
| **Action** | take the claim and clear the debris |
| **Expected** | the root holds the lock file and nothing else — the claim is the proof that the whole tree belongs to a dead process, whoever bound what is in it |

## yoke:the-trunk.07 — the claim is taken before anything is cleared

| Field | Value |
| --- | --- |
| **Cites** | specs/25.15 · arch/30-core/03 §Three things about the order that have to be argued for |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root holding a stray socket file, claimed by another process |
| **Action** | run the trunk |
| **Expected** | it stops at the claim, and the stray file is still there — clearing first would unlink the live sockets of the process that holds the instance |

## yoke:the-trunk.08 — the root's mode is the form's, whatever the umask

| Field | Value |
| --- | --- |
| **Cites** | specs/25.41 · specs/25.43 · arch/30-core/04 §Permissions are an outcome of the form |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a process umask of `000`, then of `077` |
| **Action** | claim a root in the service form and in the application form under each |
| **Expected** | the service form's root is `2750`, owned by the process's user and group; the application form's is `0700` — the directory carries the boundary, so its mode is set at creation and never taken from a umask |

## yoke:the-trunk.09 — a channel's socket takes the form's mode, and its path the ceiling

| Field | Value |
| --- | --- |
| **Cites** | specs/25.41 · specs/25.36 · arch/30-core/04 §The two ceilings · arch/30-core/04 §Permissions are an outcome of the form |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | paths of 107 and 108 characters |
| **Action** | bind a channel at each, in each form |
| **Expected** | the 107-character path binds with `0660` in the service form and `0600` in the application form; the 108-character one is refused naming its length — 108 bytes is the kernel's structure, terminator included |

## yoke:the-trunk.10 — stopping undoes the trunk in reverse, and releases the claim last

| Field | Value |
| --- | --- |
| **Cites** | specs/25.59 · specs/25.60 · arch/30-core/03 §Stopping |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a trunk that has reached readiness, with a channel bound |
| **Action** | stop it, and then claim the root from another process |
| **Expected** | what the steps set up is undone in the reverse of the order it was set up, the socket and the runtime directory are gone, and the other process takes the claim — the unlink is the signal that the instance is gone |

## yoke:the-trunk.11 — the process logger writes lines of text, governed by log.level

| Field | Value |
| --- | --- |
| **Cites** | arch/40-state/01 §What is not in either · arch/30-core/01 §core.yaml, in the service form |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with `log.level` at `info` and then at `error` |
| **Action** | start it and stop it with `SIGTERM` |
| **Expected** | at `info` its error stream holds a `key=value` line for each step it ran and one saying it is ready; at `error` it holds none — the logger is installed at step 3, and what it says is for a person reading whatever collected it |

## yoke:the-trunk.12 — the Core is ready with no channel bound, and an orderly stop leaves nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/25.21 · specs/25.22 · specs/25.59 · arch/30-core/03 §What readiness promises |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, and no surface written |
| **Action** | start it, wait for readiness, and stop it with `SIGTERM` |
| **Expected** | it reports readiness, exits zero, and its runtime directory is gone — readiness is a claim about the Core, and with no channel to bind it is reached at once |

## yoke:the-trunk.13 — a Core that died unclean leaves its debris to the next start

| Field | Value |
| --- | --- |
| **Cites** | specs/25.45 · specs/25.60 · arch/30-core/03 §Cleanup belongs to the next start |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` serving, a file left under its root, and the Core killed with `SIGKILL` |
| **Action** | start another |
| **Expected** | it serves the instance, and the root holds the lock file and nothing the dead Core or anyone left — nothing downstream depends on an orderly stop having run |
