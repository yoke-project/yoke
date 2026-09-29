<!-- Rendered by yoke-conformance from its cases: change the cases, not this file. -->
# The plugin contract, at L2

| | |
| --- | --- |
| **Feature** | the plugin contract, as the conformance suite measures it against a real Core through a family's harness |
| **Planning item** | yoke-project/yoke#19 |

## yoke:plugin.01 — the Manifest a library generates is one the Core reads

| Field | Value |
| --- | --- |
| **Cites** | specs/90.36 · specs/90.38 · specs/42.1 · arch/90-sdks/06 §The declaration a plugin library produces |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness, launched by the suite with no deployment around it |
| **Action** | `describe`, then the Manifest written into the Plugin directory of a Core that is started |
| **Expected** | a Manifest; the Core reads it, becomes ready and launches the harness as a unit, which says hello with its unit |

## yoke:plugin.02 — a registration is accepted, withholding by name what is not granted

| Field | Value |
| --- | --- |
| **Cites** | specs/50.24 · specs/50.26 · specs/50.30 · specs/90.9 · arch/50-plugin-surface/03 §Three outcomes |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness the Core launched, its plugin granted nothing |
| **Action** | `start` |
| **Expected** | `accepted with restrictions`, with every capability, stream, command and query the Manifest declares withheld, each named |

## yoke:plugin.03 — a spent token is refused at authentication, once

| Field | Value |
| --- | --- |
| **Cites** | specs/50.23 · specs/90.30 · arch/50-plugin-surface/03 §The stage travels with the refusal |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 2, admitted |
| **Action** | `start` again, with the token it was launched with |
| **Expected** | a refusal `admission.auth.consumed` at `authentication` |

## yoke:plugin.04 — nothing is emitted on a stream the Core has not activated

| Field | Value |
| --- | --- |
| **Cites** | specs/90.33 · arch/90-sdks/06 §It may not create a stream's transport |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 2, its Session open and no stream activated |
| **Action** | `emit` on the stream its Manifest declares |
| **Expected** | a refusal `stream.inactive` |

## yoke:plugin.05 — an orderly close ends the Session, and the process with it

| Field | Value |
| --- | --- |
| **Cites** | specs/50.49 · specs/90.29 · specs/50.41 · arch/90-sdks/06 §It may not hide the end of a Session |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 2, its Session open |
| **Action** | `close` |
| **Expected** | the end observed as a close the unit made, and then the harness gone |

## yoke:plugin.06 — the next life is a new process, admitted afresh

| Field | Value |
| --- | --- |
| **Cites** | specs/50.40 · specs/50.41 · specs/50.14 · arch/50-plugin-surface/04 §Identity |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 5, gone |
| **Action** | nothing, until the Core launches the unit again; then `start` |
| **Expected** | a new process saying hello with the same unit, and an acceptance |

## yoke:plugin.07 — an occurrence outside the granted scope is refused, and the Session goes on

| Field | Value |
| --- | --- |
| **Cites** | specs/50.30 · specs/50.64 · specs/50.105 · arch/50-plugin-surface/06 §Four families, closed · arch/50-plugin-surface/04 §Revocation |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 6, admitted and granted nothing |
| **Action** | `report` of the occurrence its Manifest declares, at 40 |
| **Expected** | an observation `refused` with `scope.withheld`, and no end of the Session |

## yoke:plugin.08 — a grant reaches the unit at its next admission

| Field | Value |
| --- | --- |
| **Cites** | specs/50.30 · specs/50.32 · specs/60.33 · arch/50-plugin-surface/03 §The grant is an intersection, computed once |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 7 |
| **Action** | every capability the Manifest declares granted and the unit restarted, on the administrative surface; then `start` in the life that follows |
| **Expected** | each grant effective at the next admission; the next life accepted without restriction, granted every capability, stream, command and query declared |

## yoke:plugin.09 — the Core's question reaches the unit, and its answer comes back, opaque

| Field | Value |
| --- | --- |
| **Cites** | specs/50.61 · specs/50.69 · specs/60.46 · arch/50-plugin-surface/05 §The eight · arch/60-administrative-surface/04 §The one operation whose content the Core does not read |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 8, granted everything |
| **Action** | a question of the type its Manifest declares, asked of the unit on the administrative surface with some bytes; then `answer` with other bytes |
| **Expected** | an observation `question` of that type carrying the bytes asked; the administrative answer is the bytes answered |

## yoke:plugin.10 — an occurrence is carried at the author's severity

| Field | Value |
| --- | --- |
| **Cites** | specs/50.64 · specs/50.104 · specs/90.34 · arch/50-plugin-surface/05 §The event family is where a unit declares a severity |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 8, granted everything, and a subscription to `unit.occurrence.reported` on the administrative surface |
| **Action** | `report` of the occurrence its Manifest declares, at 70, with a line |
| **Expected** | an event about the unit carrying the occurrence, severity 70 and the unit as its actor |

## yoke:plugin.11 — a health report is carried as the unit graded it

| Field | Value |
| --- | --- |
| **Cites** | specs/50.65 · specs/50.66 · specs/90.34 · arch/50-plugin-surface/05 §What a health report carries |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 8, and a subscription to `unit.condition.changed` on the administrative surface |
| **Action** | `report-health` at 80, with a line |
| **Expected** | an event about the unit at severity 80, with the unit as its actor |

## yoke:plugin.12 — disabling the plugin revokes the Session, and the process ends

| Field | Value |
| --- | --- |
| **Cites** | specs/50.49 · specs/50.51 · specs/60.29 · specs/90.29 · arch/50-plugin-surface/04 §Revocation |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 8, its Session open |
| **Action** | the plugin disabled on the administrative surface |
| **Expected** | the end observed as a revocation, the plugin disabled, and then the harness gone |
