# The guardian

| | |
| --- | --- |
| **Feature** | the account's guardian: one per account, by a claim; a socket that carries four messages and the Core's liveness; reconciling each record — the gate before every start, the Core executed with the invocation drawn from the record and `manager.conf`, a start finished at the first beat; a refusal not retried, a failure retried with a growing wait; stopping, and an ephemeral record's two endings; the boot pass; and an exit when nothing is left to watch |
| **Planning item** | yoke-project/yoke#231 |

## yoke:the-guardian.01 — one guardian per account, holding a claim and a socket only the account reaches

| Field | Value |
| --- | --- |
| **Cites** | specs/17.30 · specs/17.33 · specs/17.37 · arch/25-guardian/02 §How it comes to be running · arch/25-guardian/03 §The socket |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an account whose runtime directory is the test's own, and a record whose instance should be running on a Core that says it is alive and stays |
| **Action** | start a guardian; start a second |
| **Expected** | the first holds `guardian.lock` and binds `guardian.sock` with mode `0600`; the second exits, saying the guardian is already running as the first's process, and removes nothing |

## yoke:the-guardian.02 — a start executes the Core with the invocation the record and the configuration make, and is finished at its first beat

| Field | Value |
| --- | --- |
| **Cites** | specs/17.40 · specs/17.43 · arch/25-guardian/04 §Starting one · arch/25-guardian/02 §What it reads · arch/30-core/01 §The application form |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `com.yoke.station` `2.1.0` installed, declaring `shared.label`, required, and `shared.site` with a default; a record `bench-a` pinned to it with `shared.label` set, desired `running`; a configuration naming the install root, a Core, and a ceiling of `20MB` |
| **Action** | start the guardian; ask for the report once the Core says it is alive |
| **Expected** | the Core is executed with `--instance bench-a`, `--bundle` naming the installed bundle, `--retention bytes=20MB`, and both values — the default resolved — and without `--engine`, since nothing names an image; the report says `bench-a` is observable, with the Core's process; the record is unchanged |

## yoke:the-guardian.03 — a start the gate refuses leaves the desired state untouched, is not retried, and says why

| Field | Value |
| --- | --- |
| **Cites** | specs/17.41 · specs/17.63 · arch/25-guardian/04 §When a start is refused · arch/15-gate/06 §The phases |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the installation of `.02`; a record `bench-a` desired `running` whose `descriptor_digest` disagrees with the descriptor; a second record `bench-b`, desired `running`, whose Core says it is alive and stays |
| **Action** | start the guardian; ask for the report; tell it to reconcile `bench-a` again |
| **Expected** | no Core is executed for `bench-a`; its output names `descriptor.digest`; the report says `bench-a` is refused with that finding; its record still says `running`; told again, it runs the gate again and executes nothing |

## yoke:the-guardian.04 — a failed start is retried with a growing wait, and counts as something to watch

| Field | Value |
| --- | --- |
| **Cites** | specs/17.33 · specs/17.62 · arch/25-guardian/04 §When a start fails · arch/25-guardian/02 §How it comes to be running |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the installation of `.02`, its descriptor's restart policy a backoff of `1s` and a ceiling of `2s`; a record desired `running` whose Core exits at once |
| **Action** | start the guardian; read the report after each attempt |
| **Expected** | the Core is executed again after each failure, the report saying it is waiting, with the attempt and a wait that grows and stops at `2s`; the guardian stays running throughout |

## yoke:the-guardian.05 — stopping brings the Core down, and a guardian with nothing to watch exits

| Field | Value |
| --- | --- |
| **Cites** | specs/17.11 · specs/17.33 · arch/25-guardian/04 §Stopping one · arch/25-guardian/03 §The socket |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a guardian whose one instance is observable |
| **Action** | write `stopped` as the record's desired state, and tell the guardian to reconcile it |
| **Expected** | the Core is asked to stop and ends; the guardian exits zero and removes its socket; the record still says `stopped` and nothing else of it changed |

## yoke:the-guardian.06 — an ephemeral record goes on an orderly stop, and is marked failed and kept on a failure

| Field | Value |
| --- | --- |
| **Cites** | specs/17.28 · specs/17.29 · specs/17.43 · arch/25-guardian/04 §Stopping one · arch/25-guardian/04 §When a start fails |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an ephemeral record `run-1` whose Core says it is alive and stays; an ephemeral record `run-2` whose Core exits at once |
| **Action** | start the guardian; once `run-1` is observable, write it `stopped` and tell the guardian |
| **Expected** | `run-2` is executed once, not retried, and its record says `outcome: failed`; `run-1`'s directory is gone once its Core has stopped |

## yoke:the-guardian.07 — the boot pass brings up what autostart and the desired state both say, and never an ephemeral record

| Field | Value |
| --- | --- |
| **Cites** | specs/17.11 · specs/17.28 · arch/25-guardian/04 §What it does with a record · arch/25-guardian/07 §1 — A host comes up |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | records `a` autostart and desired `running`; `b` autostart and desired `stopped`; `c` desired `running` without autostart; `d` ephemeral, desired `running` |
| **Action** | start the guardian for the boot pass |
| **Expected** | a Core is executed for `a` and for nothing else |

## yoke:the-guardian.08 — the channel refuses a record that does not exist, and a subscription is told of each change

| Field | Value |
| --- | --- |
| **Cites** | specs/17.34 · specs/17.35 · specs/17.38 · arch/25-guardian/03 §What it carries · arch/25-guardian/03 §What carries it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a guardian whose one instance `bench-a` is observable |
| **Action** | subscribe; ask it to reconcile `nobody`; write `bench-a` stopped and ask it to reconcile `bench-a` |
| **Expected** | the subscription receives `bench-a` as observable; the request naming `nobody` is refused `record.unknown`, naming it; the subscription then receives `bench-a` stopping |

## yoke:the-guardian.09 — a beat is accepted only from the process the guardian executed for the instance

| Field | Value |
| --- | --- |
| **Cites** | specs/17.59 · arch/25-guardian/06 §The signal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a guardian whose instance `bench-a` is starting on a Core that never says it is alive |
| **Action** | from the test's own process, send a beat naming `bench-a` |
| **Expected** | it is refused, naming the instance; the report still says `bench-a` is starting |

## yoke:the-guardian.10 — a guardian starts a Core built from this tree, on its bundle, until the Core says it is alive

| Field | Value |
| --- | --- |
| **Cites** | specs/17.30 · specs/17.40 · arch/25-guardian/07 §1 — A host comes up · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built from this tree and named by the configuration, with the engine's address; `com.yoke.station` `2.1.0` installed, a unit naming an image not started automatically; a record `bench-a` pinned to it, desired `running` |
| **Action** | start the guardian; wait for the report to say `bench-a` is observable; write it stopped and tell the guardian |
| **Expected** | the Core is executed with `--engine`, becomes ready and says it is alive; the instance's state holds `version` saying `2.1.0`; once stopped the Core has ended and the guardian exits zero |
