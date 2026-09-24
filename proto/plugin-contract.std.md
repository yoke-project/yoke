# The plugin contract, at v1

| | |
| --- | --- |
| **Feature** | the definitions of the plugin surface: the two services, the registration exchange, the envelope and its eight payloads, the version the contract states and the error codes it refuses with, under a path whose segment is that version |
| **Planning item** | yoke-project/yoke#5 |

## yoke:plugin-contract.01 — the definitions build, lint clean and are formatted, with Go alone

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/05 §The encoding, and the framing · prj_structure/95 §What a contributor installs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout, with Go and `just` installed and nothing else |
| **Action** | build, lint and check the formatting of the definitions |
| **Expected** | all three succeed and nothing had to be installed beyond Go — the definitions are protocol buffers carried by gRPC, and a contributor changing them needs one toolchain, the one every other program in this repository already needs |

## yoke:plugin-contract.02 — the Go built from the definitions is committed, and current

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/40 A5 · prj_structure/97 §The definitions, and the four families that are not Go |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions and the Go generated from them, as committed |
| **Action** | regenerate the Go from the definitions |
| **Expected** | nothing changes — a Go module is fetched and never generated, so what it holds is what was committed, and a committed copy that drifted from the definitions would be a second statement of the bytes |

## yoke:plugin-contract.03 — the path's integer and the stated integer are one number

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/40 A6 · arch/00-system/05 §How a contract states its version · arch/50-plugin-surface/08 §How this contract states its version |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the plugin contract under `yoke/plugin/v1/`, and a second contract written for the test under `yoke/sample/v2/` stating the integer `3` |
| **Action** | compare each contract's path segment with the integer it states |
| **Expected** | the plugin contract states `1` and passes; the sample is rejected, naming both numbers — a contract whose two statements differ is malformed and is caught where the definitions are built, never where two parties meet |

## yoke:plugin-contract.04 — two services, and no third

| Field | Value |
| --- | --- |
| **Cites** | specs/50.2 · arch/50-plugin-surface/08 §Two services on one socket |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the plugin contract's definitions |
| **Action** | enumerate its services and their methods |
| **Expected** | exactly two: `Register`, one unary method, and `Session`, one method streaming in both directions — a unit opens one channel and lives in it, and anything else it needs is a family, never a service |

## yoke:plugin-contract.05 — the registration request carries the claim, and nothing it must not

| Field | Value |
| --- | --- |
| **Cites** | specs/50.13 · specs/50.15 · specs/50.16 · specs/50.17 · arch/50-plugin-surface/08 §The registration exchange |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the registration request's definition |
| **Action** | enumerate its fields |
| **Expected** | the plugin and unit identities, the bootstrap token, the protocol version as an integer, the artifact's version, language and SDK line, and the declared surface as four lists — capabilities, streams, commands, queries — and no address, no Session identity and no state |

## yoke:plugin-contract.06 — the registration answer says how far the request got, and what was withheld item by item

| Field | Value |
| --- | --- |
| **Cites** | specs/50.18 · specs/50.23 · specs/50.24 · specs/50.26 · arch/50-plugin-surface/08 §The registration exchange |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the registration answer's definition |
| **Action** | enumerate its fields and the values of its closed sets |
| **Expected** | an outcome of exactly three values, a stage of exactly nine, a code and a message, the Session identity, the granted scope as the same four lists, the withheld items as a list of items and never a count, and the heartbeat's interval and tolerance — the stage is a field, because whether the token was spent decides between a retry and a relaunch |

## yoke:plugin-contract.07 — the envelope carries four fields and exactly one of eight payloads

| Field | Value |
| --- | --- |
| **Cites** | specs/50.54 · specs/50.58 · specs/50.60 · specs/50.61 · specs/50.67 · arch/50-plugin-surface/08 §The envelope |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the envelope's definition |
| **Action** | enumerate its fields |
| **Expected** | the message identity, the Session identity, the sender's clock in nanoseconds and the correlation identity, and one union of exactly eight alternatives — session, control, ack, query, data, health, event, error — and no family field, no plugin, unit or incarnation, no protocol version, no sequence and no credential: the alternative that is set *is* the family |

## yoke:plugin-contract.08 — each payload carries what its family answers

| Field | Value |
| --- | --- |
| **Cites** | specs/50.102 · specs/50.103 · specs/50.104 · specs/50.65 · arch/50-plugin-surface/08 §The eight payloads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the eight payloads' definitions |
| **Action** | enumerate each one's fields and the values of its closed sets |
| **Expected** | every field `08` lists for that family and no other: a session message is `OPEN`, `CLOSE` or `REVOKED` and a revocation names one of four causes and a line; control is a command, a stream's activation naming the stream, the transport and its address, or a stream's stop; an acknowledgement is `accepted`, `done` or `failed` with a line and no result; a question and an answer; data a sequence and a payload with no stream; health a grade and a line; an event its class, severity, line and detail; an error its code, message and detail |

## yoke:plugin-contract.09 — every code this surface refuses with is stated, and none other

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/08 §This surface's error codes · arch/00-system/05 §How a refusal travels |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the codes the definitions state |
| **Action** | compare them with the twenty-three `08` lists, and each with the grammar |
| **Expected** | the same twenty-three, each dotted, lowercase and most general segment first — a code is the machine's answer and appears in a Core, in an SDK and in somebody's error handler at once, so the definitions are where every party reads it from |

## yoke:plugin-contract.10 — a refusal is a code, a message and a detail typed per code

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/05 §How a refusal travels · arch/50-plugin-surface/08 §This surface's error codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the error payload's definition |
| **Action** | enumerate its fields and the details it can carry |
| **Expected** | a code, a message and an optional detail, the detail being one of a closed set of typed messages — the divergent items of a consistency failure, the item a scope withheld — and never free text: a caller that had to parse prose would depend on wording nobody promised to keep |
