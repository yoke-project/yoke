# A container, created, attached, ended and removed

| | |
| --- | --- |
| **Feature** | what the Core asks the engine for a unit that runs in a container: created from its image's digest with what it is handed and nothing else — the instance's directory at the identical path, no network, the three labels, the instance's slice as its parent; the launching identity held constant across the boundary, per engine and mode; an image the engine does not hold reported as absent; the output attached before the container starts, its two streams kept apart; a signal, then removal; and the engine's events for this instance's containers, a container's end carrying its exit status |
| **Planning item** | yoke-project/yoke#170 |

## yoke:the-containers.01 — a container is created from its digest with what the unit is handed, and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/45.6 · specs/45.7 · specs/45.22 · specs/45.24 · arch/35-units/05 §The channel across the boundary · arch/35-units/05 §No network by default · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine recording what it is asked to create |
| **Action** | create a container for an image pinned by digest, with arguments, an environment, the instance's directory, the instance and the unit and the incarnation |
| **Expected** | the request names the image by its digest, the arguments and the environment as given, the labels `dev.yoke-project.instance`, `dev.yoke-project.unit` and `dev.yoke-project.incarnation`, the instance's directory bound at its own path and no other mount, no network, no published port, and the instance's slice as the parent |

## yoke:the-containers.02 — the launching identity is held constant across the boundary, per engine and mode

| Field | Value |
| --- | --- |
| **Cites** | specs/45.8 · specs/45.9 · specs/45.10 · arch/35-units/05 §Identity across the boundary |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | four engines: Podman rootless, Podman as root, Docker through a root-owned daemon, Docker rootless; a launching account of user 1000 and group 1000 |
| **Action** | create a container on each |
| **Expected** | Podman rootless is asked to keep the identity (`keep-id`) and given no user; Podman as root and Docker through a daemon are given user `1000:1000`; Docker rootless is given user `0:0`, which it maps to the launching account — none of the four is given the launching account's number where the engine would map it elsewhere |

## yoke:the-containers.03 — an image the engine does not hold is absent, by name, and nothing is obtained

| Field | Value |
| --- | --- |
| **Cites** | specs/45.40 · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine that holds no image of that digest |
| **Action** | create a container for it |
| **Expected** | the creation is reported as the image being absent, naming it; the engine was asked for nothing but the creation — no pull |

## yoke:the-containers.04 — the output is attached before the container starts, its two streams kept apart

| Field | Value |
| --- | --- |
| **Cites** | specs/45.23 · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose container writes two lines on its output and one on its error stream as soon as it starts |
| **Action** | attach, then start |
| **Expected** | the attachment is asked for before the start; each line arrives on the stream it was written to, in order, and the attachment ends when the output does |

## yoke:the-containers.05 — a container is signalled and removed by its identity

| Field | Value |
| --- | --- |
| **Cites** | arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine recording what it is asked |
| **Action** | signal a container with `SIGTERM`, then with `SIGKILL`, then remove it |
| **Expected** | the engine is asked to deliver each signal by name to that container, and then to remove it with what it holds |

## yoke:the-containers.06 — the engine's events for this instance's containers, a container's end carrying its exit status

| Field | Value |
| --- | --- |
| **Cites** | specs/45.25 · arch/35-units/05 §Driving the engine · arch/30-core/05 §Facts arrive; it does not go looking |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose event stream carries a container's start and end, with exit status 3, and then ends |
| **Action** | follow the events of the instance `bench` |
| **Expected** | the engine is asked for container events carrying the label `dev.yoke-project.instance=bench` alone; the end arrives with the container's identity and the status 3; when the stream ends, so does what follows it |

## yoke:the-containers.07 — an instance's slice is named after it

| Field | Value |
| --- | --- |
| **Cites** | arch/25-guardian/05 §Who creates it, and what goes in |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the instances `yoke`, `bench-a` and `lab.2` |
| **Action** | name each one's slice |
| **Expected** | `yoke-yoke.slice`, `yoke-bench\x2da.slice` and `yoke-lab.2.slice`: a `-` in the identity escaped, so every instance's slice sits directly under `yoke.slice` |

## yoke:the-containers.08 — under rootless Podman, a container runs with the launching identity, no network, and ends with its status

| Field | Value |
| --- | --- |
| **Cites** | specs/45.8 · specs/45.6 · arch/35-units/05 §Identity across the boundary · arch/35-units/05 §No network by default · arch/35-units/05 §Driving the engine |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | rootless Podman serving its API on a socket of the test's; a fixture image, built by the test over `scratch` and referenced by its digest, whose process writes its identity, its network interfaces and a file in the directory it is handed, and exits 3 |
| **Action** | follow this instance's events; create a container for the fixture with a directory of the test's; attach; start; wait for its end; remove it |
| **Expected** | the output says the launching user and group, and no interface but loopback; the file it wrote is owned by the launching user on the host; the end arrives with status 3; after removal the engine holds no container under the instance's label |
