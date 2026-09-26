# The supervisor, and the lifecycle machine

| | |
| --- | --- |
| **Feature** | the only part of the Core that touches processes, on the host: it launches a unit on the plan that verifies what it runs, hands it what it needs, captures its output, learns of its exit as it happens, restarts it on the terminal state the lifecycle machine concluded, bounds its start and stops it in reverse |
| **Planning item** | yoke-project/yoke#9 |

## yoke:the-supervisor.01 — a unit's kind decides which of the seven states it can reach

| Field | Value |
| --- | --- |
| **Cites** | specs/28.38 · specs/28.40 · arch/35-units/03 §The kind decides which of the seven a unit can reach |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a machine of each kind, and every input the machine takes |
| **Action** | drive each from `Starting` through every transition its diagram draws |
| **Expected** | a Plugin unit reaches `Admitted` on admission and `Running` when its Session opens; a unit that runs to completion and a managed interface reach `Running` when their process is up; `Admitted` and `Refused` are reached by a Plugin unit alone, and `Completed` by a unit that runs to completion alone |

## yoke:the-supervisor.02 — an exit is Completed only for a unit that runs to completion, and only with zero

| Field | Value |
| --- | --- |
| **Cites** | specs/28.39 · specs/20.28 · arch/35-units/03 §The seven states |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running unit of each kind |
| **Action** | its process exits with zero; then, for a fresh machine of each kind, with one |
| **Expected** | a unit that runs to completion is `Completed` with zero and `Failed` with one; a Plugin unit and a managed interface are `Failed` either way — ending unasked is the failure, whatever the status |

## yoke:the-supervisor.03 — a unit asked to stop has stopped, whatever its status

| Field | Value |
| --- | --- |
| **Cites** | specs/28.42 · arch/35-units/03 §The terminal states say who ended it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running unit of each kind, asked to stop |
| **Action** | its process exits with one |
| **Expected** | every kind is `Stopped` — the terminal state says who ended it |

## yoke:the-supervisor.04 — a deployment's no is Refused, and a lost Session is Failed

| Field | Value |
| --- | --- |
| **Cites** | specs/28.43 · specs/28.44 · arch/35-units/03 §The terminal states say who ended it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | Plugin units declined at the gate, withdrawn while `Admitted` and while `Running`, with a Session ended by decision, with a Session lost, and dead while `Admitted` |
| **Action** | apply each input |
| **Expected** | the first four are `Refused`, the lost Session is `Failed` with the process still alive, and the unit dead before connecting is `Failed` — a Session that ends ends the incarnation, and why it ended decides which |

## yoke:the-supervisor.05 — a terminal state ends the incarnation, and nothing after it moves the machine

| Field | Value |
| --- | --- |
| **Cites** | specs/28.41 · specs/28.32 · arch/35-units/03 §One authority |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit whose process has exited |
| **Action** | apply the unit's own report that it is running, an admission, and a Session opening |
| **Expected** | it stays where the exit put it — a report from a dead process describes a moment that has passed, and a new state belongs to a new incarnation |

## yoke:the-supervisor.06 — a report carries a condition and never moves the state

| Field | Value |
| --- | --- |
| **Cites** | specs/28.36 · specs/28.37 · arch/35-units/03 §Three things that look like states and are not |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running unit that has reported nothing; then the same unit reporting a grade of ninety and a line |
| **Action** | read its state and its condition each time |
| **Expected** | it is `Running` both times; first with no condition, then with that grade and that line — silence is not a claim, and a unit grading itself badly is running with a condition |

## yoke:the-supervisor.07 — the restart policy reads the terminal state and the kind

| Field | Value |
| --- | --- |
| **Cites** | specs/28.46 · specs/28.47 · specs/20.24 · arch/35-units/03 §The restart policy reads the state and is not part of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | every terminal state, for every kind, with and without `restart.on_failure` |
| **Action** | ask whether each restarts |
| **Expected** | `Failed` restarts a Plugin unit and a managed interface, and a unit that runs to completion only when its declaration says so; `Stopped`, `Completed` and `Refused` never restart under any declaration |

## yoke:the-supervisor.08 — the identity is verified on the descriptor that is then executed

| Field | Value |
| --- | --- |
| **Cites** | specs/28.50 · arch/35-units/04 §The plan |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a host unit declared with its executable's digest, and the same unit declared with a digest that disagrees |
| **Action** | launch each |
| **Expected** | the first runs; the second never starts, reaches `Failed` as an ordinary failed attempt, and the report names the digest that disagreed |

## yoke:the-supervisor.09 — the process is handed what it needs and nothing else

| Field | Value |
| --- | --- |
| **Cites** | arch/35-units/04 §What the Core hands the process · arch/50-plugin-surface/01 §What the launch supplies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin unit and a unit that runs to completion, each declaring an argument holding a space and a variable of its own |
| **Action** | launch each, and read what its process received |
| **Expected** | each has `YOKE_UNIT`, `YOKE_SOCKET` at the instance's plugin channel, `YOKE_BIND` at `plugins/<unit>.sock` under the root and a `YOKE_TOKEN` issued for it; the Plugin unit alone has `YOKE_PLUGIN`; the declared variable is added; the argument arrives as one element |

## yoke:the-supervisor.10 — a unit's output is captured line by line, with the incarnation that wrote it

| Field | Value |
| --- | --- |
| **Cites** | specs/20.13 · arch/35-units/04 §Capturing output |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit that writes two lines to its output and one to its error stream, and then fails and is launched again |
| **Action** | let it run twice |
| **Expected** | every line is captured, each with the unit and the incarnation that wrote it, and the two lives' lines are told apart by incarnation |

## yoke:the-supervisor.11 — an exit reaches the machine when it happens

| Field | Value |
| --- | --- |
| **Cites** | specs/20.18 · arch/30-core/05 §Facts arrive; it does not go looking |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a managed interface whose process exits after a moment |
| **Action** | wait for its state |
| **Expected** | it is `Failed` within a second of the exit, told by the kernel's report of the child ending |

## yoke:the-supervisor.12 — the wait doubles to its ceiling, and attempts never run out

| Field | Value |
| --- | --- |
| **Cites** | specs/20.25 · specs/20.27 · arch/30-core/05 §The restart policy |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a managed interface that fails at once, a first wait of 50 ms and a ceiling of 200 ms |
| **Action** | let it fail repeatedly |
| **Expected** | the waits run 50, 100, 200, 200 ms and attempts continue past the ceiling; every attempt is a new incarnation with a new token |

## yoke:the-supervisor.13 — waiting for the next attempt is reported, and is not a state

| Field | Value |
| --- | --- |
| **Cites** | specs/20.29 · arch/30-core/05 §The restart policy |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit waiting for its next attempt |
| **Action** | read its status |
| **Expected** | its state is the `Failed` its last incarnation reached, and the status says it is waiting, on which attempt, and when the next one is due |

## yoke:the-supervisor.14 — a unit that stayed ready for the stability window starts counting again

| Field | Value |
| --- | --- |
| **Cites** | specs/20.26 · arch/30-core/05 §The restart policy |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a managed interface that has failed twice, then runs longer than the stability window before failing again |
| **Action** | read the wait before its next attempt |
| **Expected** | it is the first wait again — a count is per episode, measured from readiness |

## yoke:the-supervisor.15 — a unit that never becomes ready is ended at the startup window

| Field | Value |
| --- | --- |
| **Cites** | specs/20.30 · arch/30-core/05 §One window bounds the start |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin unit whose process runs and is never admitted, with a startup window of 300 ms |
| **Action** | wait past the window |
| **Expected** | its process is gone and it is `Failed` — the window bounds `Starting`, and disposing of a process going nowhere is the supervisor's |

## yoke:the-supervisor.16 — stopping asks in reverse order, waits, and then ends what does not go

| Field | Value |
| --- | --- |
| **Cites** | specs/20.31 · arch/35-units/04 §Ending one · arch/30-core/05 §Stopping |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three running units launched in order, the second forking a child and the third ignoring the termination signal, with a stop window of 300 ms |
| **Action** | stop the supervisor |
| **Expected** | they are asked to stop third, second, first; the third is ended after the window; the forked child is gone with its unit; all three are `Stopped` |

## yoke:the-supervisor.17 — a supervisor that cannot observe concludes nothing, and says so

| Field | Value |
| --- | --- |
| **Cites** | specs/20.20 · specs/20.21 · specs/20.22 · specs/20.23 · arch/30-core/05 §When the source of facts goes quiet |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running unit, and the backend's source of facts going quiet at a known instant |
| **Action** | read the unit; ask to stop it; launch another; then let the source return |
| **Expected** | the unit is still `Running`, carrying the condition that it is not observable since that instant; stopping it fails naming the backend; the new launch is an ordinary failed attempt; when the source returns the condition is gone and the state is what it was |
