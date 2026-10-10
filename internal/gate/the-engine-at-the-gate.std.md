# The engine at the gate

| | |
| --- | --- |
| **Feature** | the gate's checks for a container engine: where a unit names an image, an engine that cannot be reached refused and a root-owned one reported as weaker; where none does, no engine looked for; and a containerised unit's bound path checked against what its launch will map |
| **Planning item** | yoke-project/yoke#174 |

## yoke:the-engine-at-the-gate.01 — where a unit names an image, an unreachable engine is refused and a root-owned one is weaker

| Field | Value |
| --- | --- |
| **Cites** | specs/45.38 · specs/45.30 · specs/45.31 · arch/15-gate/06 §Phase 4 — host facts · arch/15-gate/06 §Phase 5 — weaker arrangements |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition with a containerised oneshot; a host whose engine cannot be reached, then one whose engine is rootless, then one whose engine runs as root |
| **Action** | check it at the start moment against each |
| **Expected** | `engine.unreachable`, a refusal naming what the engine answered; then nothing; then `engine.rootful`, weaker, and the deployment passes |

## yoke:the-engine-at-the-gate.02 — where no unit names an image, nothing looks for an engine

| Field | Value |
| --- | --- |
| **Cites** | specs/45.39 · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition whose units all run on the host, and a host whose engine records every time it is asked; then the containerised composition at the composing moment |
| **Action** | check the first at the start moment, the second while composing |
| **Expected** | both pass, and the engine was never asked |

## yoke:the-engine-at-the-gate.03 — a containerised unit's bound device is checked against what its launch will map

| Field | Value |
| --- | --- |
| **Cites** | specs/15.29 · specs/45.12 · arch/15-gate/06 §Phase 4 — host facts · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a containerised interface needing a device bound to a path that does not exist on the host, and a reachable rootless engine |
| **Action** | check it at the start moment |
| **Expected** | `path.missing` naming the binding and the path: the launch maps every bound device at its own path, so what it will map is what the host holds |
