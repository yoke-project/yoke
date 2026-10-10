# The engine at the start

| | |
| --- | --- |
| **Feature** | what the Core does with the gate's findings about an engine when it starts: an engine that cannot be reached is the refusal step 7 fails on, named; a root-owned one is reported and repeated whenever the instance is read |
| **Planning item** | yoke-project/yoke#174 |

## yoke:the-engine-at-the-start.01 — an engine the gate cannot reach is the refusal step 7 fails on

| Field | Value |
| --- | --- |
| **Cites** | specs/45.38 · specs/25.17 · arch/15-gate/06 §Phase 4 — host facts · arch/30-core/03 §Three things about the order that have to be argued for |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition with a containerised oneshot, and `engine` naming a socket nothing serves |
| **Action** | start the Core |
| **Expected** | the Core exits with a failure naming step 7, the code `engine.unreachable` and the engine's address, and is never ready |

## yoke:the-engine-at-the-start.02 — a root-owned engine is reported at the start and repeated when the instance is read

| Field | Value |
| --- | --- |
| **Cites** | specs/45.30 · specs/45.31 · arch/15-gate/06 §Phase 5 — weaker arrangements · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition with a containerised oneshot, and `engine` naming a socket served by an engine that answers as a root-owned daemon and holds nothing |
| **Action** | start the Core, and read the instance on the operator projection |
| **Expected** | the Core is ready, having said `engine.rootful` as a warning; the instance's record lists `engine.rootful` among its weaker arrangements |
