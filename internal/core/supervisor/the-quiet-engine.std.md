# The engine gone quiet, and its return

| | |
| --- | --- |
| **Feature** | the second supervisory backend when its facts stop arriving: the engine's events ending is the engine going quiet, each unit on it keeping its state and carrying the condition that it is not currently observable, with the instant it began and a grade the Core gives it; an operation that needs the engine a fault naming it, a launch an ordinary failed attempt that tries to reach it; the return learnt from a notification and never on a timer, and answered by reconciling against the instance's label; the backend each unit runs on, read |
| **Planning item** | yoke-project/yoke#171 |

## yoke:the-quiet-engine.01 — the engine's events ending is the engine going quiet, and nothing is concluded

| Field | Value |
| --- | --- |
| **Cites** | specs/20.20 · specs/20.21 · specs/20.23 · specs/45.26 · arch/30-core/05 §When the source of facts goes quiet · arch/35-units/03 §Three things that look like states and are not · arch/45-events/06 §`unit` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine running a containerised unit of kind `interface`, and a unit on the host beside it, both `Running` |
| **Action** | end the engine's event stream; read both units |
| **Expected** | the containerised unit is still `Running` on its first incarnation, and carries the condition that it is not currently observable, graded `30` by the Core, with a line naming the container engine and the instant the stream ended; a `unit.condition.changed` is published for it with the Core as its actor and that grade; the unit on the host carries no condition and nothing is published about it; each unit's backend is named, `container` and `host` |

## yoke:the-quiet-engine.02 — while the engine is quiet, an operation that needs it is a fault naming it

| Field | Value |
| --- | --- |
| **Cites** | specs/20.22 · specs/20.23 · arch/30-core/05 §When the source of facts goes quiet · arch/60-administrative-surface/07 §This surface's codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose event stream has ended, under a running containerised unit, with a running unit on the host beside it |
| **Action** | stop, start and restart the containerised unit; stop the unit on the host; then stop the supervisor |
| **Expected** | each of the three operations fails as the backend being unreachable, naming the `container` backend, and the engine is asked nothing; the containerised unit is still `Running`; the unit on the host is `Stopped`; the supervisor's stop stops what it can reach and fails naming the containerised unit |

## yoke:the-quiet-engine.03 — a launch while the engine is quiet is an ordinary failed attempt, and the attempt after its return launches

| Field | Value |
| --- | --- |
| **Cites** | specs/20.22 · specs/20.25 · arch/30-core/05 §When the source of facts goes quiet · arch/35-units/03 §The restart policy reads the state and is not part of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose event stream has ended and which refuses to be followed again until the test lets it; nothing tells the supervisor it returned |
| **Action** | launch a containerised unit of kind `interface`; then let the engine be followed again |
| **Expected** | the unit is `Failed` and waits, its failure naming the container engine, and nothing is created; once the engine can be followed, a later attempt follows it, reconciles, creates, attaches and starts the container, the unit is `Running`, and the condition is gone |

## yoke:the-quiet-engine.04 — the engine's return is answered by reconciling against the instance's label

| Field | Value |
| --- | --- |
| **Cites** | specs/20.20 · specs/20.23 · specs/45.23 · specs/45.24 · specs/45.26 · arch/30-core/05 §When the source of facts goes quiet · arch/30-core/08 §3 — The engine goes away, and comes back · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three containerised units running — one oneshot, two interfaces — and the engine's event stream ended; meanwhile the oneshot's container ended with status 0, the first interface's kept running, the second interface's is gone, and a container this supervisor never launched carries the instance's label |
| **Action** | tell the supervisor the engine returned |
| **Expected** | the events are followed again before the engine is asked what runs under the label; the oneshot is `Completed` and its container removed; the first interface is still `Running` on its first incarnation, nothing has signalled it, and a line its container writes afterwards reaches the output under that incarnation; the second interface is `Failed`; the container nobody launched is removed; every condition is gone, and a `unit.condition.changed` by the Core says so for each unit |

## yoke:the-quiet-engine.05 — the return is learnt from a notification, and never looked for on a timer

| Field | Value |
| --- | --- |
| **Cites** | specs/20.18 · specs/20.21 · specs/45.25 · arch/30-core/05 §Facts arrive; it does not go looking · arch/30-core/05 §When the source of facts goes quiet |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running containerised unit, an engine that refuses to be followed while it is away, and whose return is a notice the test gives |
| **Action** | end the engine's event stream; wait well past the supervisor's backoff with the engine away; then let the engine be followed and give the notice |
| **Expected** | after the one attempt to follow it again that the stream's end makes, nothing is asked of the engine while it is away, and the condition carries the same instant throughout; after the notice, the events are followed and the engine is asked what runs under the label, and the condition is gone |

## yoke:the-quiet-engine.06 — a unit read while its engine is quiet shows its backend and the condition

| Field | Value |
| --- | --- |
| **Cites** | specs/20.20 · specs/20.21 · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a containerised unit `Running` while its engine is quiet, carrying the condition the supervisor gives it, and a unit on the host |
| **Action** | read both units' records |
| **Expected** | the containerised unit's backend is `container`, its state `Running`, and its condition the grade, the line and the instant the supervisor gave; the host unit's backend is `host` |

## yoke:the-quiet-engine.07 — under rootless Podman, a container that ended while the engine was away is concluded on its return

| Field | Value |
| --- | --- |
| **Cites** | specs/20.20 · specs/20.23 · specs/45.24 · specs/45.26 · arch/30-core/05 §When the source of facts goes quiet · arch/30-core/08 §3 — The engine goes away, and comes back |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | rootless Podman serving its API on a socket the Core is configured with; a fixture image, built by the test over `scratch` and referenced by its digest, whose process waits a few seconds and exits 0; a composition with one unit of kind `oneshot` naming it |
| **Action** | start the Core and wait for the unit to run; kill the API service; wait for the container to end; serve the API on the same socket again; then stop the Core |
| **Expected** | while the service is away the unit carries the condition and is not concluded; after it returns the condition is cleared, the unit is `Completed` on its first incarnation and no second one is launched; once the Core has stopped, the engine holds no container under the instance's label |
