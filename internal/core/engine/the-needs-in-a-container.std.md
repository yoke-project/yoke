# The needs, as the engine is asked for them

| | |
| --- | --- |
| **Feature** | what a container is created with for its declared needs, per engine and mode: devices at their paths, mounts at their paths, the devices' groups as the engine can carry them — kept by the runtime on rootless Podman, and a network only where asked |
| **Planning item** | yoke-project/yoke#173 |

## yoke:the-needs-in-a-container.01 — devices, mounts, groups and the network, per engine and mode

| Field | Value |
| --- | --- |
| **Cites** | specs/45.10 · specs/45.11 · specs/45.12 · specs/45.17 · arch/35-units/05 §What each declared need expands into · arch/35-units/05 §Identity across the boundary |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a launch with a device, two mounts, the device's group `20`, and a network; and one with none of them; an engine answering as rootless Podman, rootless Docker, and Podman running as root |
| **Action** | create each launch on each engine |
| **Expected** | the first is created with the device mapped at its own path with `rwm`, the instance's directory and both mounts bound at their own paths, and no `NetworkMode`; its groups are none added and the runtime annotation `run.oci.keep_original_groups` set on rootless Podman, none on rootless Docker, and `20` on Podman as root; the second has only the instance's directory, no device, no groups and `NetworkMode` `none` |
