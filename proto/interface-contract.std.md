# The interface contract

| | |
| --- | --- |
| **Feature** | the definitions of the interface surface at `v1`: one service, `Interface`, with one bidirectional method, carrying one operation union; the eleven operations and their answers; the frames an attachment is made of, the Core's first carrying the opening picture; the records a read, a snapshot and the opening picture are made of, over the three subject kinds a channel observes; where a stream's data arrives, and the frame that carries it on the connection; the version every request states; and this surface's codes, each refusal a code, a message and a typed detail |
| **Planning item** | yoke-project/yoke#136 |

## yoke:interface-contract.01 — one service with one bidirectional method, and no second

| Field | Value |
| --- | --- |
| **Cites** | specs/70.1 · specs/70.11 · arch/70-interface-surface/08 §`local` — the typed projection · arch/70-interface-surface/02 §`local` — the typed projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the interface contract's definitions |
| **Action** | enumerate its services and their methods |
| **Expected** | `Interface` with `Attach`, a stream of frames in each direction, from the client's frame to the Core's; no other service and no other method |

## yoke:interface-contract.02 — the union holds the eleven operations, and every request states the version

| Field | Value |
| --- | --- |
| **Cites** | specs/70.12 · arch/70-interface-surface/04 §The eleven · arch/70-interface-surface/08 §One union, two projections · arch/70-interface-surface/08 §How this contract states its version · arch/00-system/05 §How a contract states its version |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of `Request` and `Response`, and the contract's version |
| **Action** | enumerate the members of each union, and read the version |
| **Expected** | `Request` carries a version integer and one member per operation — `authenticate`, `read`, `subscribe`, `confirm`, `command`, `query`, `stream.start`, `stream.stop`, `stream.subscribe`, `stream.unsubscribe` and `reclaim` — and `Response` one answer per operation, in the same order; no member names a plugin or an incarnation, and none carries a client identity; the contract states 1, at the path `v1` |

## yoke:interface-contract.03 — an attachment's frames, and the opening picture first

| Field | Value |
| --- | --- |
| **Cites** | specs/70.9 · specs/70.7 · arch/70-interface-surface/08 §`local` — the typed projection · arch/70-interface-surface/03 §The opening picture · arch/70-interface-surface/03 §Who the client is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of the two frames, of the opening and of a completion |
| **Action** | enumerate each frame's fields and union, the opening's fields and a completion's |
| **Expected** | a client's frame carries a call identity and either a request or a cancellation naming a call; the Core's carries a call identity and one of an opening, an answer, an event, a stream delivery, a refusal or a completion; the opening carries the picture as a snapshot, the standing subscription's call and the version the Core speaks; a completion says whether the caller cancelled or the Core ended the call |

## yoke:interface-contract.04 — a read, a snapshot and the opening picture are one record, over three subject kinds

| Field | Value |
| --- | --- |
| **Cites** | specs/70.8 · specs/70.48 · arch/70-interface-surface/05 §One model, not two · arch/70-interface-surface/05 §What a channel observes, and what it may address · arch/70-interface-surface/05 §Three subject kinds, and three it does not see |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of a record, of a snapshot and of the answer to `read` |
| **Action** | enumerate a record's union; read a unit's and a channel's records |
| **Expected** | a record is one of `instance`, `unit` and `channel`, and of no other kind; a unit's carries its declared identity and kind, what is observed of it — state, incarnation, the moment of the last transition, its condition and the streams activated — and what a channel may address on it: the streams, commands and questions granted; a channel's carries its name, projection, address class and cardinality, and whether it is attached and by which client, whether it is suspended, in which grade, by which channel and for which reason; a read's answer and a snapshot both hold records, the snapshot with the sequence it was taken at |

## yoke:interface-contract.05 — a command and a question carry an opaque payload, and come back correlated

| Field | Value |
| --- | --- |
| **Cites** | specs/70.4 · arch/70-interface-surface/04 §What is opaque, and what that costs the Core · arch/70-interface-surface/04 §The eleven |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of a command, a question, and their answers |
| **Action** | read their fields |
| **Expected** | a command and a question each name a unit and a type and carry bytes; a command's answer is the unit's acknowledgement — accepted, done or failed — with its line, and a question's is the unit's bytes; nothing in either names a channel |

## yoke:interface-contract.06 — where a stream's data arrives, and the frame that carries it on the connection

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §The three delivery paths · arch/70-interface-surface/07 §What every path preserves · arch/70-interface-surface/08 §`local` — the typed projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of the answer to `stream.subscribe` and of a stream delivery |
| **Action** | read their fields and the answer's union |
| **Expected** | the answer names the delivery and says where its data arrives — a socket path, the delivery on this connection, or a browser path — and whether the stream is flowing; a stream delivery carries the delivery, the sequence, the sender's clock at emission and the payload |

## yoke:interface-contract.07 — every code this surface refuses with is stated, and none other

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/08 §This surface's error codes · arch/00-system/05 §How a refusal travels |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the interface contract's enumeration of codes |
| **Action** | read each value's dotted name |
| **Expected** | the seventeen codes of this surface, each under the grammar, and no other |

## yoke:interface-contract.08 — a refusal is a code, a message and a typed detail

| Field | Value |
| --- | --- |
| **Cites** | specs/70.27 · arch/00-system/05 §How a refusal travels · arch/70-interface-surface/06 §What each grade withdraws |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definition of a refusal |
| **Action** | read its fields and its detail's union |
| **Expected** | a code and a message, both strings, and a detail that is one of: the subject named — its kind and its identity; the item withheld or undeclared; or the suspension — its grade and the channel that prevails |
