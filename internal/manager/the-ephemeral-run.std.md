# The ephemeral run

| | |
| --- | --- |
| **Feature** | `yoke app run`: running an application that has no record creates an ephemeral one, visibly so and never eligible for autostart, which the guardian executes like every other start and the manager follows until it is interrupted; an orderly stop takes the record with it and a failure leaves it marked failed until reclaimed; `--from` runs a bundle being written, unverified, and the instance says so of itself; `--admit-unlaunched` is a second waiver asked for separately, and both belong to the invocation |
| **Planning item** | yoke-project/yoke#229 |

## yoke:the-ephemeral-run.01 — a run with no record creates an ephemeral one, and an interrupt takes it away

| Field | Value |
| --- | --- |
| **Cites** | specs/17.27 · specs/17.28 · specs/17.29 · specs/15.68 · arch/10-manager/07 §Running an application that was never declared |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke`, `yoke-core` and `yoke-guardian` built from this tree, the guardian on the `PATH`; `com.yoke.station` `2.1.0` installed; no instance declared and no guardian running |
| **Action** | run `yoke app run com.yoke.station` as a process of its own; once it says the instance is observable, run `yoke instance list` and read the record; then interrupt it |
| **Expected** | the record says `ephemeral: true`, `autostart: false`, desired state `running`, pinned to `2.1.0`, with no source and no waiver; the list names the instance observable and `ephemeral`; after the interrupt the run exits 0 saying the instance stopped, its directory is gone, its claim is free, and the guardian has exited |

## yoke:the-ephemeral-run.02 — a run that fails leaves its record, marked failed, until a reclaiming pass

| Field | Value |
| --- | --- |
| **Cites** | specs/17.29 · specs/17.43 · specs/17.57 · arch/10-manager/07 §Running an application that was never declared · arch/10-manager/06 §Reclaiming |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | as in `.01`, a run of `com.yoke.station` followed until it is observable |
| **Action** | kill the instance's Core; then run `yoke instance list`; then `yoke app prune com.yoke.station` |
| **Expected** | the run exits 1 saying the instance failed and that its record is kept; the record is there with `outcome: failed` and its desired state still `running`; the list names it `failed`; the prune names it as removed, and its directory is gone |

## yoke:the-ephemeral-run.03 — a bundle being written runs unverified, and the instance says so of itself

| Field | Value |
| --- | --- |
| **Cites** | specs/15.70 · specs/25.64 · arch/10-manager/07 §Running a bundle that is being written · arch/10-manager/08 §3 — A run from a working directory |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | as in `.01`, and a working directory holding a bundle of `com.yoke.bench` `0.1.0`, installed nowhere |
| **Action** | run `yoke app run --from <dir>`; once observable, read the record and run `yoke core status --instance` with its name; then interrupt it |
| **Expected** | the run says the directory is not verified; the record names the directory as its source, `verification: waived`, no admission waiver and no descriptor digest; the status names `bundle.unverified` at the directory among the instance's weaker arrangements; after the interrupt the run exits 0 and the record is gone |

## yoke:the-ephemeral-run.04 — the second waiver is asked for separately, and only an invocation carries either

| Field | Value |
| --- | --- |
| **Cites** | specs/15.71 · specs/50.38 · arch/10-manager/07 §Running a bundle that is being written |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `com.yoke.station` `2.1.0` installed, and a working directory holding a bundle whose descriptor carries `verification: waived` beside its identity |
| **Action** | create an ephemeral record for `com.yoke.station` with the unlaunched waiver; run `yoke instance create com.yoke.station --admit-unlaunched`; run `yoke app run --from` the directory |
| **Expected** | the record names `admission: unlaunched-permitted`, and no source and no verification waiver; the create exits 2 naming the flag and writes no record; the run exits 2, the descriptor refused for a key it does not know, and writes no record |

## yoke:the-ephemeral-run.05 — what a run cannot be is refused before anything is written

| Field | Value |
| --- | --- |
| **Cites** | specs/17.27 · arch/10-manager/07 §Running an application that was never declared · arch/10-manager/01 §What it reports |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `com.yoke.station` `2.1.0` installed; no instance declared |
| **Action** | run `yoke app run` with no subject; with `com.yoke.other`; with `--from` and an application's identifier; with `--from` a directory holding no descriptor |
| **Expected** | each exits 2 and no instance directory exists afterwards |
