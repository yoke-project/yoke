# The administrative operations, refusals, and the stop

| | |
| --- | --- |
| **Feature** | the operations that change something, each against the subject its name takes — policy per plugin, execution per unit — and terminating in the Core; the answer every change carries: what it replaced, when it takes effect and one consequence per unit affected; an effect already true answered as success; every change recorded against its actor; a question carried to a unit and its answer carried back, opaque, bounded, and waited for 30 s; the contract's version checked on every request; this surface's refusals, each a code and a typed detail; and, while the instance stops, changes refused with `instance.stopping` and reads still answered |
| **Planning item** | yoke-project/yoke#94 |

## yoke:the-operations.01 — every request states the contract's version, and one this Core does not speak is refused

| Field | Value |
| --- | --- |
| **Cites** | arch/60-administrative-surface/08 §How this contract states its version · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving `plugin.enable` |
| **Action** | issue it stating no version, then the version 2 |
| **Expected** | both are refused with `compat.unsupported` |

## yoke:the-operations.02 — disabling a plugin answers what it replaced, and revokes each of its live Sessions

| Field | Value |
| --- | --- |
| **Cites** | specs/60.29 · specs/60.31 · specs/60.32 · specs/60.34 · arch/60-administrative-surface/04 §What every answer carries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin declared and enabled, two of whose units are running with a Session, in their lives 3 and 5 |
| **Action** | disable it; then disable it again |
| **Expected** | the first answers that it was enabled, effective immediately, with one consequence per unit naming it, its life and that its Session was revoked; both Sessions were revoked as the plugin being disabled; the Registry holds a decision naming the operator, and `plugin.policy.changed` was published with the operator as its actor; the second succeeds, answers that it was disabled, and has no consequence and no decision |

## yoke:the-operations.03 — a grant is checked against the Manifest, and reaches a unit at its next admission

| Field | Value |
| --- | --- |
| **Cites** | specs/60.26 · specs/60.28 · specs/60.33 · arch/60-administrative-surface/04 §What every answer carries · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same plugin, whose Manifest declares `stream.data.publish`, with one unit running |
| **Action** | grant `stream.data.publish`; grant `head.move`; grant to a plugin nobody declared; grant to the unit's identity as if it were a plugin |
| **Expected** | the first answers that it was not granted, effective at the unit's next admission, with the running unit as a consequence still under the scope of its admission; the second is refused with `capability.undeclared`, its detail naming `head.move`; the third with `subject.unknown`, its detail naming the kind `plugin` and the identity; the fourth with `subject.wrong_kind` |

## yoke:the-operations.04 — stopping, starting and restarting a unit answer the state replaced and the lives ended and begun

| Field | Value |
| --- | --- |
| **Cites** | specs/60.26 · specs/60.30 · specs/60.44 · arch/60-administrative-surface/04 §What every answer carries · arch/60-administrative-surface/04 §Every operation terminates in the Core |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit running in its life 2, supervised |
| **Action** | stop it; start it; start it again; restart it; stop a unit nobody declared; stop it while the backend cannot be reached |
| **Expected** | the stop answers `Running`, effective immediately, with a consequence naming life 2 as ended; the start answers `Stopped`, with a consequence naming the life begun; the second start succeeds, answering `Running` with no consequence; the restart names the life ended and the one begun; the unknown unit is refused with `subject.unknown`; the unreachable backend with `backend.unavailable` |

## yoke:the-operations.05 — a retention override is written to the log store, and answers the one it replaced

| Field | Value |
| --- | --- |
| **Cites** | specs/60.28 · arch/60-administrative-surface/04 §The four names, applied · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared unit with no override |
| **Action** | set an override with a limit of zero entries; set one of 7 days and 1000 entries; set one with no limit; clear it |
| **Expected** | the first is refused with `retention.invalid`; the second answers an unconstrained policy, and the log store holds 7 days and 1000 entries for the unit; the third answers 7 days and 1000 entries, and is legal; the clear answers the unconstrained one it removed, and the log store holds no override |

## yoke:the-operations.06 — a question is carried to the unit and its answer carried back, opaque, and recorded without either

| Field | Value |
| --- | --- |
| **Cites** | specs/60.45 · specs/60.46 · arch/60-administrative-surface/04 §The one operation whose content the Core does not read · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit running in its life 4 with a Session that answers `status` with the bytes it was asked, reversed; a unit running with no Session; a unit stopped; a unit whose Session never answers, with the wait at 100 ms |
| **Action** | ask the first `status` with some bytes; with 1 MiB and one byte; ask each of the others |
| **Expected** | the first answer is the bytes reversed, and the log store holds an entry naming the operator, the unit and life 4, holding neither the question nor the answer; the oversized question is refused with `operation.malformed`; the others with `unit.no_session`, `unit.not_running` and `unit.unanswered`, each naming the unit in its detail |

## yoke:the-operations.07 — every change is recorded against its actor, and an effect already true records nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/60.14 · arch/60-administrative-surface/03 §Where the identity lands |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin declared and disabled, and a unit running |
| **Action** | enable the plugin; enable it again; stop the unit |
| **Expected** | the log store holds one entry for the enablement and one for the stop, each naming the operator by the person the channel established; the second enablement added none |

## yoke:the-operations.08 — while the instance stops, a change is refused naming the stop, and a read is answered

| Field | Value |
| --- | --- |
| **Cites** | specs/60.55 · specs/60.56 · specs/60.57 · arch/60-administrative-surface/07 §While the instance is stopping |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving `plugin.enable`, `unit.ask` and `read`, with the instance stopping |
| **Action** | issue each |
| **Expected** | `plugin.enable` and `unit.ask` are refused with `instance.stopping`; `read` is answered |

## yoke:the-operations.09 — stream control waits for streams on their own transports

| Field | Value |
| --- | --- |
| **Cites** | specs/60.47 · arch/60-administrative-surface/04 §Every operation terminates in the Core · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Core's operations |
| **Action** | list the operations it serves; issue `unit.stream.start` |
| **Expected** | it serves the nine other changes and `unit.ask`, and not `unit.stream.start` or `unit.stream.stop`, which is refused with `operation.unknown`: a stream travels on a transport of its own, and no such transport exists before 0.3 |

## yoke:the-operations.10 — through the Core, an operator disables a plugin and the decision is theirs

| Field | Value |
| --- | --- |
| **Cites** | specs/60.14 · specs/60.34 · arch/60-administrative-surface/04 §What every answer carries · arch/60-administrative-surface/03 §Where the identity lands |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and started in the service form, with one Manifest in its Plugin directory |
| **Action** | disable the plugin by `Call`; disable it again on a shell connection |
| **Expected** | the first answers that it was enabled; the second that it was disabled; the Core's output records `plugin.policy.changed` about the plugin, with the operator as its actor, naming the account the test runs under |
