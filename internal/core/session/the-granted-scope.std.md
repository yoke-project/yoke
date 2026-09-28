# The granted scope, carried to every message

| | |
| --- | --- |
| **Feature** | what admission grants — the intersection of what the Manifest declares and what the Registry authorises, for each kind of object a capability governs, occurrences included — carried to the unit's Session for its whole life and enforced there on every message it governs, by exact membership: an occurrence the unit reports, and a command, a question or a stream the Core would send it |
| **Planning item** | yoke-project/yoke#84 |

## yoke:the-granted-scope.01 — an occurrence a unit reports is received only if it was granted

| Field | Value |
| --- | --- |
| **Cites** | specs/50.64 · specs/50.30 · arch/50-plugin-surface/05 §The event family is where a unit declares a severity · arch/50-plugin-surface/08 §This surface's error codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session whose scope declares the occurrences `calibration.drift` and `head.fault` and grants only the first |
| **Action** | the unit reports `calibration.drift` at severity 90, `head.fault`, and `lamp.failure` |
| **Expected** | the first is received with no answer; the second is answered `scope.withheld` and the third `scope.undeclared`, each correlated to it; the Session stays open — the severity a unit attaches widens nothing |

## yoke:the-granted-scope.02 — the Core sends nothing the grant does not cover

| Field | Value |
| --- | --- |
| **Cites** | specs/50.30 · specs/50.78 · arch/50-plugin-surface/06 §Four families, closed · arch/50-plugin-surface/03 §The grant is an intersection, computed once |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session whose scope declares the commands `calibrate` and `zero`, the question `range` and `status`, and the streams `station.spectra` and `station.diagnostics`, and grants the first of each |
| **Action** | the Core is asked to send `zero`, `purge`, the question `status`, the question `uptime`, and an activation of `station.diagnostics`; then `calibrate`, `range` and an activation of `station.spectra` |
| **Expected** | the first five are refused where they were asked for — `scope.withheld` for what is declared, `scope.undeclared` for what is not — and nothing reaches the unit; the last three arrive |

## yoke:the-granted-scope.03 — enforcement is exact membership

| Field | Value |
| --- | --- |
| **Cites** | specs/50.82 · arch/50-plugin-surface/06 §Hierarchy is for writing policy, not for evaluating it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session whose scope grants the command `calibrate` and the occurrence `calibration.drift` |
| **Action** | the Core is asked to send `calibrate.full` and `Calibrate`; the unit reports `calibration` and `calibration.drift.fast` |
| **Expected** | each is refused as undeclared — no prefix, no case folding and no hierarchy is evaluated when a message is checked |

## yoke:the-granted-scope.04 — admission hands the Session its scope, which a later policy change does not reach

| Field | Value |
| --- | --- |
| **Cites** | specs/50.30 · specs/50.32 · arch/50-plugin-surface/03 §The grant is an intersection, computed once |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin whose Manifest declares a command, a query, a stream and two occurrences, each governed by a capability, with the command's and the first occurrence's authorised |
| **Action** | admit a unit and read the scope its Session identity resolves to; authorise the rest; read it again; release the unit and admit it again |
| **Expected** | the first scope grants the command and the first occurrence, and declares everything; after the policy change the same Session's scope is unchanged; the next admission's grants everything |

## yoke:the-granted-scope.05 — through the Core, an occurrence nobody authorised is refused and recorded

| Field | Value |
| --- | --- |
| **Cites** | specs/50.64 · arch/50-plugin-surface/05 §The event family is where a unit declares a severity |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a plugin that declares the occurrence `calibration.drift` and was granted nothing, and a composition running one unit of it that registers, opens its Session and reports the occurrence |
| **Action** | start the Core |
| **Expected** | the unit receives `scope.withheld` correlated to its report, and the Core's output records the refusal |
