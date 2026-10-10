# A unit in a container, supervised

| | |
| --- | --- |
| **Feature** | the supervisor's second way of launching a unit: a unit of kind `oneshot` or `interface` that names an image runs in a container the engine creates, handed what it would be handed on the host, its output captured by incarnation, its end learnt from the engine's events and concluded by the same lifecycle machine; an image the engine does not hold, or an engine not reached, a fault on which the restart policy waits; a stop that signals, waits the stop window, ends, and removes |
| **Planning item** | yoke-project/yoke#170 |

## yoke:the-container-backend.01 — a unit naming an image is launched in a container, handed what it would be handed on the host

| Field | Value |
| --- | --- |
| **Cites** | specs/45.1 · specs/45.6 · arch/35-units/05 §What containerisation is allowed to change · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine recording what it is asked, whose container writes a line and keeps running |
| **Action** | launch a unit of kind `oneshot` naming an image, with arguments and an environment of its own |
| **Expected** | the engine is asked to create a container from that image with the unit's arguments, the environment the supervisor hands a unit on the host, the instance's root as the directory, the instance, the unit and its incarnation, and the launching account's identity; it is attached before it is started; the line reaches the output under the unit and its incarnation; the unit is `Running`, as a oneshot whose process started is |

## yoke:the-container-backend.02 — the engine's report of a container's end is the unit's exit, and the container is removed

| Field | Value |
| --- | --- |
| **Cites** | specs/45.25 · arch/35-units/05 §Driving the engine · arch/30-core/05 §Facts arrive; it does not go looking |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose containers end on their own, the first with status 0 and the second with status 1; the second unit restarts on failure |
| **Action** | launch two units of kind `oneshot` naming an image |
| **Expected** | the first is `Completed` and the second `Failed`, waiting to be launched again; each container is removed once its end is concluded |

## yoke:the-container-backend.03 — an image the engine does not hold is a fault, and the restart policy waits on it

| Field | Value |
| --- | --- |
| **Cites** | specs/45.40 · arch/35-units/05 §Driving the engine · arch/35-units/03 §The terminal states say who ended it · arch/35-units/03 §The restart policy reads the state and is not part of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine that holds no image, until the test gives it one |
| **Action** | launch a unit of kind `interface` naming an image; then let the engine hold it |
| **Expected** | the unit is `Failed`, never `Refused`, with a failure naming the image, and waits; nothing is obtained; once the engine holds the image the next attempt is created and started |

## yoke:the-container-backend.04 — a stop signals the container, waits the stop window, ends it, and removes it

| Field | Value |
| --- | --- |
| **Cites** | arch/35-units/05 §Driving the engine · arch/35-units/03 §The restart policy reads the state and is not part of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose container ignores `SIGTERM` |
| **Action** | launch a unit naming an image, then stop it |
| **Expected** | the container is sent `SIGTERM`, then `SIGKILL` once the stop window has passed; the unit is `Stopped`; the container is removed before the stop returns |

## yoke:the-container-backend.05 — with no engine reached, launching in a container is a fault that names it

| Field | Value |
| --- | --- |
| **Cites** | specs/45.38 · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a supervisor given no engine |
| **Action** | launch a unit of kind `interface` naming an image, and a unit on the host beside it |
| **Expected** | the unit is `Failed` and waits, its failure saying that no container engine is reached; a unit on the host beside it is launched as ever |

## yoke:the-container-backend.06 — the Core runs a oneshot in a container under the environment's engine

| Field | Value |
| --- | --- |
| **Cites** | specs/45.1 · specs/45.8 · specs/45.23 · arch/35-units/05 §Driving the engine · arch/35-units/05 §Identity across the boundary |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the environment's engine serving its API on a socket the Core is configured with — rootless Podman on a socket of the test's, or the Docker daemon; a fixture image, built by the test over `scratch` and referenced by its digest, that says who it runs as and exits 0; a composition with one unit of kind `oneshot` naming it |
| **Action** | start the Core, and wait for the unit to complete; then stop the Core |
| **Expected** | the unit's line reaches the Core's log under its first incarnation and says the launching user; the unit is `Completed`; once the Core has stopped, the engine holds no container under the instance's label |
