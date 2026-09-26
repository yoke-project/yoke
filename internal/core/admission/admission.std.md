# Admission

| | |
| --- | --- |
| **Feature** | the single gate between a running process and a participant: a registration on the instance's plugin channel, decided in nine ordered stages against the request, the Manifest as read at this start, the Registry and the bootstrap token, with three outcomes, a grant computed once as the intersection of what is declared and what is authorised, what is recorded, and the development waiver |
| **Planning item** | yoke-project/yoke#15 |

## yoke:admission.01 — nine stages in order, and a refusal names the stage it stopped at

| Field | Value |
| --- | --- |
| **Cites** | specs/50.9 · specs/50.18 · specs/50.19 · specs/50.23 · specs/50.25 · arch/50-plugin-surface/03 §The nine stages · arch/50-plugin-surface/03 §The stage travels with the refusal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment composing unit `acquire` of plugin `com.yoke.station.acquire`, declared and enabled, its token issued; and a request for each stage made to fail in turn — no unit; a plugin the Registry does not hold; a unit nothing composes; the plugin disabled; a token that is not the one issued; protocol 7; a declared stream the Manifest does not declare; `acquire` already admitted |
| **Action** | register each |
| **Expected** | each is refused, with the stage and code `structural`/`admission.structural.malformed`, `identity`/`admission.identity.unknown_plugin`, `identity`/`admission.identity.unknown_unit`, `administrative state`/`admission.state.disabled`, `authentication`/`admission.auth.invalid`, `compatibility`/`admission.compat.unsupported`, `declaration consistency`/`admission.consistency.divergent` and `unit conflict`/`admission.conflict.unit_live`, and a message for a person |

## yoke:admission.02 — a plugin an operator disabled is refused without its credential being examined

| Field | Value |
| --- | --- |
| **Cites** | specs/50.20 · arch/50-plugin-surface/03 §The nine stages |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composed unit whose plugin is disabled, its token issued |
| **Action** | register it; enable the plugin; register again with the same token |
| **Expected** | the first is refused at the administrative state; the second is accepted — the token was not spent by a refusal that came before it |

## yoke:admission.03 — a token is good once, for its unit, within that unit's startup window

| Field | Value |
| --- | --- |
| **Cites** | specs/50.12 · specs/50.13 · specs/50.7 · arch/50-plugin-surface/03 §The four inputs · arch/50-plugin-surface/01 §The startup window, and its three uses |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two composed units of one plugin, `a` with a startup window of 200 ms and `b` of the default; a token issued to each; and a unit `c` of another plugin |
| **Action** | register `b` with `a`'s token; `b` naming `c`'s plugin with `b`'s token; `b` with its own, twice; `a` with its own after 300 ms; then `a` again with a token issued for a new launch |
| **Expected** | `admission.auth.invalid`, `admission.auth.invalid`, then accepted and `admission.auth.consumed`, then `admission.auth.expired`, then accepted |

## yoke:admission.04 — from stage 5 a refusal has spent the token

| Field | Value |
| --- | --- |
| **Cites** | specs/50.23 · arch/50-plugin-surface/03 §The stage travels with the refusal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composed unit, its token issued |
| **Action** | register with protocol 7; then register correctly with the same token |
| **Expected** | the first is refused at compatibility; the second is refused at authentication, `admission.auth.consumed` — recovery is a relaunch, never a retry |

## yoke:admission.05 — the declared surface matches the Manifest as read at this start

| Field | Value |
| --- | --- |
| **Cites** | specs/50.11 · specs/50.16 · specs/42.13 · arch/50-plugin-surface/03 §What the request claims |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest declaring two streams, a command, a query and their four capabilities |
| **Action** | register declaring the four lists in another order; then with one capability missing; then with a query the Manifest does not declare |
| **Expected** | the first is accepted; the other two are refused at declaration consistency |

## yoke:admission.06 — the grant is what is declared and authorised, and each withheld item is named

| Field | Value |
| --- | --- |
| **Cites** | specs/50.24 · specs/50.26 · specs/50.29 · specs/50.30 · specs/50.33 · arch/50-plugin-surface/03 §The grant is an intersection, computed once · arch/50-plugin-surface/03 §Three outcomes · arch/97-trace/03 §The intersection does something here |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest declaring the streams `station.spectra` and `station.diagnostics` with a capability each; the Registry granting `stream.spectra.publish` and a capability the Manifest does not declare; then granting both declared |
| **Action** | register |
| **Expected** | accepted with restrictions: granted `stream.spectra.publish` and `station.spectra`; withheld `stream.diagnostics.publish` and `station.diagnostics`, named; the undeclared grant appears nowhere. With both granted: accepted, nothing withheld |

## yoke:admission.07 — stage 7 never refuses

| Field | Value |
| --- | --- |
| **Cites** | specs/50.22 · specs/50.28 · arch/50-plugin-surface/03 §The nine stages |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin with no grant, as every newly declared plugin is |
| **Action** | register |
| **Expected** | accepted with restrictions, with an empty grant and every declared item withheld |

## yoke:admission.08 — one policy, several grants, and a conflict is about a unit

| Field | Value |
| --- | --- |
| **Cites** | specs/50.21 · specs/50.33 · arch/50-plugin-surface/03 §The nine stages · arch/50-plugin-surface/03 §The grant is an intersection, computed once |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two composed units of one plugin, `acquire-1` and `acquire-2`, each with its token |
| **Action** | register both |
| **Expected** | both are accepted, with the same grant and two different Session identities |

## yoke:admission.09 — an acceptance carries an unguessable Session identity and the Core's heartbeat terms

| Field | Value |
| --- | --- |
| **Cites** | specs/50.17 · specs/50.31 · arch/50-plugin-surface/03 §Three outcomes · arch/50-plugin-surface/04 §Identity |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composed unit whose policy sets `heartbeat.interval` to 4 s and `heartbeat.tolerance` to 5 |
| **Action** | register it |
| **Expected** | the Session identity is 43 characters of base64url — 32 random bytes; the heartbeat terms are 4 s and 5 |

## yoke:admission.10 — what admission records, and what it never does

| Field | Value |
| --- | --- |
| **Cites** | specs/50.15 · specs/50.34 · specs/50.35 · specs/50.36 · specs/50.37 · arch/50-plugin-surface/03 §What admission records |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two composed units of one plugin |
| **Action** | register one with version `2.1.0`, language `go` and SDK `yoke-sdk-go 1.4.2`; register the other with a wrong token |
| **Expected** | the Registry records the version, language and SDK; the process logger holds a line for each outcome naming the unit, and for the acceptance how many items were withheld; the refusal's line marks that it reached the credential; no line holds either token |

## yoke:admission.11 — the development waiver relaxes authentication and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/50.38 · arch/50-plugin-surface/03 §The development waiver · arch/30-core/01 §What it is executed with |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | admission with the waiver, and admission without it |
| **Action** | with the waiver, register a composed unit with no token, a unit nothing composes, a disabled plugin's unit and a divergent declaration; without it, a composed unit with no token |
| **Expected** | with the waiver the first is accepted and the other three are refused at identity, administrative state and consistency; without it, refused at authentication |

## yoke:admission.12 — the outcome reaches the lifecycle machine when it is the unit's

| Field | Value |
| --- | --- |
| **Cites** | specs/50.101 · arch/35-units/03 §The seven states · arch/50-plugin-surface/03 §The stage travels with the refusal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three composed units, each with its token, and what admission tells the machine collected |
| **Action** | register the first correctly; the second with protocol 7; the third with a wrong token |
| **Expected** | the first unit's machine is told it was accepted, the second's that it was declined; nothing is told about the third — a request that did not prove its provenance says nothing about the unit it names |

## yoke:admission.13 — a launched unit registers on the plugin channel and is admitted

| Field | Value |
| --- | --- |
| **Cites** | specs/50.9 · specs/50.3 · arch/50-plugin-surface/01 §The order of the first four acts · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a Plugin directory holding one Manifest, its executable a program that registers on `YOKE_SOCKET` with `YOKE_TOKEN` and writes the answer, and a composition running one unit of it |
| **Action** | start the Core |
| **Expected** | the plugin channel is bound at the root with the form's mode; the unit's output says it was accepted with restrictions, with a Session identity of 43 characters; the Core's output says the unit was admitted |
