# A managed interface in a container

| | |
| --- | --- |
| **Feature** | a managed interface whose unit names an image: it reaches the local channel the Core bound for it from inside its container, with no network and no port; and when its container stops, its channel is observed as detached and arbitration is recomputed, nothing staying suspended in favour of a channel nobody holds |
| **Planning item** | yoke-project/yoke#175 |

## yoke:a-managed-interface-in-a-container.01 — from inside its container, a managed interface attaches to its local channel, and binds nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/45.18 · specs/45.17 · specs/28.6 · arch/35-units/05 §No network by default · arch/35-units/05 §Arbitration, a container, and two subjects |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the environment's engine serving its API; a composition whose managed interface names a fixture image that attaches to the address it is handed, serving the local channel `front` |
| **Action** | start the Core |
| **Expected** | `channel.attached` is published for `front`; the interface's container has no network and publishes no port |

## yoke:a-managed-interface-in-a-container.02 — when its container stops, its channel is detached and arbitration recomputed

| Field | Value |
| --- | --- |
| **Cites** | specs/45.36 · arch/35-units/05 §Arbitration, a container, and two subjects · arch/70-interface-surface/03 §What ends an attachment |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | case 01's deployment, with a second local channel `bench` over which `front` prevails, and a client attached to `bench`, which is suspended |
| **Action** | stop the interface's container with the engine's own tools |
| **Expected** | `channel.detached` is published for `front`, and then `channel.resumed` for `bench`: nothing stays suspended in favour of a channel nobody holds |
