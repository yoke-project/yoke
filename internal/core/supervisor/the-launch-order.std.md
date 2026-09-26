# The launch order

| | |
| --- | --- |
| **Feature** | the order the Core derives from what each unit depends on: every unit with no unmet dependency launched at once, each other one at the moment its last dependency became ready by the definition its kind fixes; a dependency that never arrives, bounded by the dependent's startup window, leaves the dependent not started — in no state, the instance not failed — and reported with the chain back to the unit that actually failed |
| **Planning item** | yoke-project/yoke#54 |

## yoke:the-launch-order.01 — what depends on nothing unmet is launched at once

| Field | Value |
| --- | --- |
| **Cites** | specs/28.16 · arch/35-units/02 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three units given to the supervisor together: `a` and `b`, each serving and depending on nothing, and `c` depending on `a`, which is a unit that runs to completion and never becomes ready while it serves |
| **Action** | start them |
| **Expected** | when starting returns, `a` and `b` each already have an incarnation, neither having waited for the other; `c` has no incarnation and no state, and is awaiting `a` |

## yoke:the-launch-order.02 — a dependent of a Plugin unit is launched when its Session opens

| Field | Value |
| --- | --- |
| **Cites** | specs/28.18 · specs/28.19 · arch/35-units/02 §Ready is three different things |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin unit `acquire` that serves, and `archive` depending on it |
| **Action** | start them; tell the supervisor `acquire` was admitted; then that its Session opened |
| **Expected** | after the admission `archive` is still awaiting `acquire` with no incarnation; once the Session opened, `archive` is launched |

## yoke:the-launch-order.03 — a dependent of a unit that runs to completion is launched after it exits zero

| Field | Value |
| --- | --- |
| **Cites** | specs/28.18 · specs/28.19 · specs/45.27 · arch/35-units/02 §Ready is three different things |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit that runs to completion, `provision`, which exits zero after 200 ms, and `store` depending on it |
| **Action** | start them |
| **Expected** | while `provision` is running, `store` is awaiting it with no incarnation; once `provision` is `Completed`, `store` is launched |

## yoke:the-launch-order.04 — a dependency that never arrives: not started, not fatal, and the chain named

| Field | Value |
| --- | --- |
| **Cites** | specs/28.21 · specs/31.10 · arch/35-units/02 §A dependency that never arrives |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `provision`, a unit that runs to completion and exits 1; `store` depending on it and `archive` depending on `store`, each with a startup window of 300 ms; `other`, serving and depending on nothing; and a supervisor told of every unit not started |
| **Action** | start them, and wait a second |
| **Expected** | `other` is running; `store` and `archive` have no incarnation and no state; the supervisor was told of each once, `store` with `store did not start because provision exited 1` and `archive` with `archive did not start because store did not start because provision exited 1`, and each unit's status carries the same |

## yoke:the-launch-order.05 — a dependency the instance does not start never arrives

| Field | Value |
| --- | --- |
| **Cites** | specs/28.21 · specs/28.17 · arch/35-units/02 §A dependency that never arrives |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `store`, with a startup window of 300 ms, depending on `later`, a unit that is not among those started |
| **Action** | start `store` alone, and wait a second |
| **Expected** | `store` was not started, reported as `store did not start because later does not start with the instance` |

## yoke:the-launch-order.06 — units stop in the reverse of the order they were launched, and a wait ends with the stop

| Field | Value |
| --- | --- |
| **Cites** | specs/20.31 · arch/35-units/02 §The order · arch/35-units/04 §Ending one |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `c` depending on `a`, given first; `a`, a unit that runs to completion and exits zero; `b`, serving and depending on nothing; and `d`, with a startup window of 500 ms, depending on `c`, a Plugin unit that serves and is never admitted |
| **Action** | start them; once `c` has an incarnation, stop the supervisor, and wait a second |
| **Expected** | the stop asked `c` before `b` — the order of launching reversed, not the order of giving; `d` was never launched and is not reported as not started, because the instance stopped before its window ran out |

## yoke:the-launch-order.07 — the Core launches in the order it derives, and reports the chain

| Field | Value |
| --- | --- |
| **Cites** | specs/28.16 · specs/28.21 · specs/20.10 · arch/35-units/02 §The order · arch/35-units/02 §A dependency that never arrives |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition of units that run to completion: `second`, echoing a line and depending on `first`, echoing a line after 300 ms; `provision`, exiting 1; `store`, depending on `provision`, and `archive`, depending on `store`, each with a startup window of 1 s |
| **Action** | start the Core, and read what it says for five seconds |
| **Expected** | `first`'s line comes before `second`'s; the Core reports `archive` not started with the cause `archive did not start because store did not start because provision exited 1`, and `store` with its own; the Core is still running |
