# The migration step

| | |
| --- | --- |
| **Feature** | `migrates_from` evaluated by the Core: the stamp read once at step 10 before it is rewritten, and a oneshot whose range does not admit the version found there, or that finds none, skipped — no process, no incarnation, straight to `Completed`, its dependants starting, its one event carrying `skipped` |
| **Planning item** | yoke-project/yoke#314 |

## yoke:the-migration-step.01 — a skipped unit is `Completed` without a launch, and what depends on it starts

| Field | Value |
| --- | --- |
| **Cites** | specs/12.28 · specs/14.20 · specs/28.9 · arch/35-units/03 §The seven states · arch/45-events/06 §`unit` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two units given to the supervisor together: `migrate`, a unit that runs to completion marked skipped for the range `<2.2` and the version `2.2.0`, whose program would write a line, and `store`, depending on `migrate`, which serves |
| **Action** | start them |
| **Expected** | `migrate` is `Completed` with no incarnation, and its program wrote no line; exactly one `unit.state.changed` names `migrate`, with no incarnation, no `from`, `to` `Completed` and `skipped` holding the range and the version; `store` is launched |

## yoke:the-migration-step.02 — the range is weighed against the version that last ran, and no version is not admitted

| Field | Value |
| --- | --- |
| **Cites** | specs/14.20 · arch/30-core/03 §The eleven steps · arch/35-units/03 §The seven states |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit that runs to completion declaring `migrates_from: "<2.2"`, a Plugin unit declaring none, and the versions `2.1.0`, `2.2.0` and no stamp |
| **Action** | weigh each unit against each version |
| **Expected** | with `2.1.0` nothing is skipped; with `2.2.0` the oneshot is skipped for `<2.2` and `2.2.0`; with no stamp it is skipped for `<2.2` and `none`; the Plugin unit is never skipped |

## yoke:the-migration-step.03 — a Core on a bundle runs the migration once, across the version it rewrites

| Field | Value |
| --- | --- |
| **Cites** | specs/12.28 · specs/14.20 · specs/28.9 · arch/30-core/03 §The eleven steps · arch/40-state/05 §The stamp · arch/97-trace/06 §The update itself |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built; a bundle at `2.2.0` declaring `migrate`, a oneshot with `migrates_from: "<2.2"` appending a line to a file, and `store`, a oneshot depending on it appending another; an instance whose state carries the stamp `2.1.0` |
| **Action** | start the Core on the bundle until `store` completes, stop it, and start it again; then clear the instance's state and start it once more |
| **Expected** | the first start runs `migrate` then `store`, and leaves the stamp `2.2.0`; the second skips `migrate`, its event carrying `skipped` with `2.2.0`, and runs `store`; after the clear `migrate` is skipped with `none` and `store` runs; `migrate`'s line is in the file exactly once |
