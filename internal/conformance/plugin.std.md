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
