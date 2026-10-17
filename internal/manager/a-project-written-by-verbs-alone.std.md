# A project written by verbs alone

| | |
| --- | --- |
| **Feature** | the first of the composer's flows end to end with the built programs: a project made by `yoke-compose init`, every declaration in it written by an authoring verb or a block — no descriptor edited by hand — checked, an advisory item waived with a reason, built into a package, and that package installed, instantiated and started by `yoke`, running what the verbs declared with the values they declared |
| **Planning item** | yoke-project/yoke#285 |

## yoke:a-project-written-by-verbs-alone.01 — a project written by verbs alone is built, installed and runs what was declared

| Field | Value |
| --- | --- |
| **Cites** | specs/95.1 · specs/95.7 · arch/20-composer/06 §1 — Composing an application · arch/20-composer/01 §The verbs · arch/20-composer/03 §Placing one · arch/20-composer/04 §Waiving an item · arch/10-manager/08 §1 — From a package to a running instance |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-compose`, `yoke`, `yoke-core` and `yoke-guardian` built from this tree; an account whose install root is empty and whose host keeps no trust store; a program `components/probe` that writes its first argument to the file `$OUT` names; a block `writer` whose oneshot unit runs that program with `${line}` and `OUT` set to `${target}`, its group declaring both; the program and the block are the only files written by hand |
| **Action** | `yoke-compose init com.yoke.composed --version 1.0.0`; `add parameter probe.greeting` and `add parameter shared.out`, each with a default; `add unit probe --kind oneshot --exec components/probe --arg ${greeting}`; `set units.probe.env.OUT ${out}`; `set units.probe.policy.restart.on_failure true`; `block add` the block `--as second`, then `set` the defaults of its `line` and its `target`; `check`; `waive` what it advises with a reason; `check` again; `build`; then `yoke app install` the package, `yoke instance create com.yoke.composed --name bench`, `yoke instance start bench`; then `yoke instance stop bench` |
| **Expected** | every verb exits `0`; the first `check` refuses nothing and advises `oneshot.restart.unchecked` at `units.probe.policy.restart.on_failure`; the second advises nothing and counts one waived item; `build` names a package; the install names the application at `1.0.0`; once started the instance is listed observable, the file the probe was given holds the greeting and the file the placed unit was given holds its `line`; the stop leaves the instance's claim free |
