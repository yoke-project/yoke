# The Core as the centre

| | |
| --- | --- |
| **Feature** | what makes the Core the one centre and no more: it holds nothing tied to any unit — no import of a unit's code, no unit's identity written into it, no branch on a payload it carries — and nothing a unit sends can address another unit or a client, every message reaching the Core and going no further on the unit's say |
| **Planning item** | yoke-project/yoke#340 |

## yoke:the-centre.01 — no package of the Core imports a unit's code or names a unit

| Field | Value |
| --- | --- |
| **Cites** | specs/00.11 · specs/00.23 · arch/00-system/03 §The list · arch/30-core/01 §One process per instance |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | this tree |
| **Action** | list every package `yoke-core` is built from, and read the Core's sources that are not tests |
| **Expected** | every package is this module's, the Go standard library's, or a third party's that is not a unit's — none from the reference programs, and none whose path names a domain; no source names the identity of a Plugin the reference publishes |

## yoke:the-centre.02 — what a unit sends names no destination, so it reaches the Core and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/00.15 · arch/00-system/03 §The list · arch/50-plugin-surface/04 §The envelope carries four fields |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the plugin contract's definitions |
| **Action** | read the fields of the envelope a unit sends |
| **Expected** | its header is the message's identity, the Session's, the time it was sent and what it answers, and its payload one of the eight families; no field names a unit, a channel, a client or any other destination |
