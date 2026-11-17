# A stream nobody reads any more

| | |
| --- | --- |
| **Feature** | what becomes of a stream started by a client of a channel when that client goes away: a stream is the unit's, not the client's that asked for it, so it keeps flowing after its starter detached, with nobody subscribed, until a stop — from any client of a channel allowed to start it, or from an administrative caller — or the end of the unit's Session |
| **Planning item** | yoke-project/yoke#463 |

## yoke:a-stream-nobody-reads.01 — through the Core, a stream outlives the client that started it, and a stop from another ends it

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §Two acts, and they are not the same one · arch/70-interface-surface/03 §What ends an attachment |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a local channel open to several clients and a Plugin unit of a program that emits on activation and acknowledges a stop; on the operator projection, the stream's capability granted and the unit restarted |
| **Action** | a client attaches, starts the stream without subscribing, and goes away; once `channel.detached` is logged, a second client attaches and subscribes; it stops the stream; a third attaches and subscribes |
| **Expected** | the second is answered that the stream is flowing; the stop is acknowledged; the third is answered that it is not |
