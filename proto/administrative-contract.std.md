# The administrative contract

| | |
| --- | --- |
| **Feature** | the definitions of the administrative surface at `v1`: two services carrying one operation union — `Operator` with a unary call and a stream of answers, `Shell` with one stream of frames in each direction; the sixteen operations and their answers, every change answering what it replaced, when it takes effect and what it did to each unit; the records a read and a snapshot are made of; the frame a held connection adds; the version every request states; and this surface's codes, each refusal a code, a message and a typed detail |
| **Planning item** | yoke-project/yoke#91 |

## yoke:administrative-contract.01 — two services over one union, and no third

| Field | Value |
| --- | --- |
| **Cites** | specs/60.1 · specs/60.18 · specs/60.20 · arch/60-administrative-surface/08 §Two services over one union · arch/60-administrative-surface/02 §One contract, two shapes of exchange |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the administrative contract's definitions |
| **Action** | enumerate its services and their methods |
| **Expected** | `Operator` with `Call`, unary, and `Watch`, a stream of answers to one request; `Shell` with `Connect`, a stream of frames each way; `Call` and `Watch` take the one `Request` and answer the one `Response`, and no third service exists |

## yoke:administrative-contract.02 — the union holds the sixteen operations, and every request states the version

| Field | Value |
| --- | --- |
| **Cites** | specs/60.26 · specs/60.27 · specs/60.59 · arch/60-administrative-surface/04 §The four names, applied · arch/60-administrative-surface/08 §How this contract states its version · arch/00-system/05 §How a contract states its version |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of `Request` and `Response`, and the contract's version |
| **Action** | enumerate the members of each union, and read the version |
| **Expected** | `Request` carries a version integer and one member per operation — the four on a plugin, the seven on a unit, `read`, `log.query`, `log.follow` and `subscribe` — and `Response` one answer per operation, in the same order; the contract states 1, at the path `v1` |

## yoke:administrative-contract.03 — every change answers what it replaced, when it takes effect, and what it did to each unit

| Field | Value |
| --- | --- |
| **Cites** | specs/60.31 · specs/60.33 · specs/60.34 · specs/60.35 · arch/60-administrative-surface/04 §What every answer carries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the answer the eleven changing operations share |
| **Action** | read its fields and those of a consequence |
| **Expected** | it carries what was previously true, whether the change is effective immediately or at the unit's next admission, and a list of consequences, each naming a unit, its incarnation and what happened to it; no request anywhere carries a unit's state |

## yoke:administrative-contract.04 — a read and a snapshot are made of one record, one kind per subject

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.37 · specs/60.42 · arch/60-administrative-surface/05 §One model, not two · arch/60-administrative-surface/05 §Which subjects are readable · arch/60-administrative-surface/05 §Authorised and observed, in one answer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of a record, of a snapshot and of the answer to `read` |
| **Action** | enumerate a record's union; read a plugin's and a unit's records |
| **Expected** | a record is one of the six subject kinds; a plugin's groups its fields as declared, authorized and observed, and a unit's carries its declared and observed groups and the record of the plugin it runs; a read's answer and a snapshot both hold records, the snapshot with the sequence it was taken at |

## yoke:administrative-contract.05 — the frame a held connection adds

| Field | Value |
| --- | --- |
| **Cites** | specs/60.19 · specs/60.23 · arch/60-administrative-surface/08 §The frame a held connection adds · arch/60-administrative-surface/06 §Who subscribes differs; what is promised does not |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of the two frames |
| **Action** | enumerate each frame's fields and union, and the opening's fields |
| **Expected** | a client's frame carries a call identity and either a request or a cancellation naming a call; the Core's carries a call identity and one of an opening, an answer, an event, a refusal or a completion; the opening names the connection, the actor the channel established, the standing subscription's call and the version the Core speaks |

## yoke:administrative-contract.06 — every code this surface refuses with is stated, and none other

| Field | Value |
| --- | --- |
| **Cites** | specs/60.57 · arch/60-administrative-surface/07 §This surface's codes · arch/00-system/05 §How a refusal travels |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the administrative contract's enumeration of codes |
| **Action** | read each value's dotted name |
| **Expected** | the fifteen codes of this surface, each under the grammar, and no other |

## yoke:administrative-contract.07 — a refusal is a code, a message and a typed detail

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/05 §How a refusal travels · arch/60-administrative-surface/07 §The shape is the corpus's |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definition of a refusal |
| **Action** | read its fields and its detail's union |
| **Expected** | a code and a message, both strings, and a detail that is one of: the subject named — its kind and its identity — or the item that was withheld or undeclared |
