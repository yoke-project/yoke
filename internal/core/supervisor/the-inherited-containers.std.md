# Containers inherited at startup

| | |
| --- | --- |
| **Feature** | the trunk's step 7: every container under the instance's label, running or ended, stopped and removed before the declarations are read, each one named; an engine that cannot be asked, or a container that cannot be removed, fatal; and where no unit names an image, no engine looked for and nothing that can fail |
| **Planning item** | yoke-project/yoke#172 |

## yoke:the-inherited-containers.01 — every container under the instance's label is removed, running or ended, and named

| Field | Value |
| --- | --- |
| **Cites** | specs/25.14 · specs/25.17 · specs/45.24 · arch/30-core/03 §The eleven steps · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding, under the instance's label, a container that still runs and one that ended with status 1, each with its unit and incarnation |
| **Action** | clear what the instance inherited |
| **Expected** | the engine is asked once what runs under the instance's label, and both containers are removed; what is returned names each one with its unit, its incarnation and whether it was running; nothing is created, started or followed |

## yoke:the-inherited-containers.02 — an engine that cannot be asked, or a container that cannot be removed, is an error naming it

| Field | Value |
| --- | --- |
| **Cites** | specs/25.17 · arch/30-core/03 §Three things about the order that have to be argued for |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | first an engine that cannot be reached; then one holding two containers under the label, the first of which it refuses to remove |
| **Action** | clear what the instance inherited, once against each |
| **Expected** | the first is an error naming the engine as unreachable; the second is an error naming the container it refused, and the other container is still asked to be removed |

## yoke:the-inherited-containers.03 — a Core started after one that died removes what it left, before its declarations, and nothing of another instance

| Field | Value |
| --- | --- |
| **Cites** | specs/25.14 · specs/25.17 · specs/45.24 · arch/30-core/03 §The eleven steps · arch/30-core/03 §Cleanup belongs to the next start · arch/35-units/05 §Driving the engine |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | rootless Podman serving its API; a Core whose composition runs a containerised oneshot that does not end, killed with `SIGKILL` once the unit is `Running`, so its container still runs; a container of the same image under another instance's label |
| **Action** | start the Core again on the same configuration |
| **Expected** | before the line that step 7 is done, a line names the first life's container with its unit, its incarnation `1` and that it was running; that container no longer exists; the other instance's container still runs; the unit is launched as incarnation `2` |

## yoke:the-inherited-containers.04 — an engine that cannot be reached where a unit names an image is fatal

| Field | Value |
| --- | --- |
| **Cites** | specs/25.17 · specs/25.18 · specs/45.38 · arch/30-core/03 §Three things about the order that have to be argued for · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition with a containerised oneshot, and `engine` naming a socket nothing serves |
| **Action** | start the Core |
| **Expected** | the Core exits with a failure naming step 7 and the engine's address; the instance is never ready, and nothing is launched |

## yoke:the-inherited-containers.05 — where no unit names an image, no engine is looked for and step 7 cannot fail

| Field | Value |
| --- | --- |
| **Cites** | specs/25.17 · specs/45.39 · arch/30-core/03 §Three things about the order that have to be argued for |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition whose one unit is a oneshot on the host, and `engine` naming a socket nothing serves |
| **Action** | start the Core |
| **Expected** | step 7 is done, the instance is ready, and the unit completes; nothing is said about the container engine |
