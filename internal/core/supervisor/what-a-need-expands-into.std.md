# What a need expands into

| | |
| --- | --- |
| **Feature** | each declared need expanded at every launch, from the Core's environment and the host's filesystem, on the host and in a container: a device's bound path checked and mapped, a storage directory created and mounted, a display and an audio path found and handed, a network given only where asked; a need that cannot be expanded a fault of that launch |
| **Planning item** | yoke-project/yoke#173 |

## yoke:what-a-need-expands-into.01 — a device bound to a path that does not exist is a fault of the launch

| Field | Value |
| --- | --- |
| **Cites** | specs/45.16 · arch/35-units/05 §What each declared need expands into · arch/35-units/03 §The restart policy reads the state and is not part of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a oneshot on the host needing a device bound to a path that exists, and another needing one bound to a path that does not |
| **Action** | launch both |
| **Expected** | the first completes; the second is `Failed`, its failure naming the need and the path, and no process was started for it |

## yoke:what-a-need-expands-into.02 — a storage need is a directory the Core creates under the instance's state, and the unit is told where

| Field | Value |
| --- | --- |
| **Cites** | specs/14.17 · specs/14.18 · specs/45.12 · arch/15-gate/03 §`units` · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition whose oneshot needs `storage:datasets` and names `${bind.datasets}` in its arguments; an instance state directory with no `storage/` in it |
| **Action** | resolve the deployment's units, and launch the oneshot |
| **Expected** | the argument is `<state>/storage/datasets`; before the process starts the directory exists, mode `0700`, owned by the launching account; a second launch finds it as it was, with what the first wrote still in it |

## yoke:what-a-need-expands-into.03 — a display is the host's own, found in the Core's environment and handed to the unit

| Field | Value |
| --- | --- |
| **Cites** | specs/45.16 · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Core's environment naming a Wayland socket by name under its runtime directory, an X11 display `:7` whose socket exists, and an authority file; then an environment naming neither; a unit on the host needing `display` |
| **Action** | launch the unit under each |
| **Expected** | under the first it is handed `WAYLAND_DISPLAY` as the socket's absolute path, `XDG_RUNTIME_DIR`, `DISPLAY=:7` and `XAUTHORITY`, and nothing else of the Core's environment; under the second it is `Failed`, its failure naming the display |

## yoke:what-a-need-expands-into.04 — an audio path is PipeWire's socket or PulseAudio's, whichever exists

| Field | Value |
| --- | --- |
| **Cites** | specs/45.16 · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a runtime directory holding `pipewire-0` and `pulse/native`; then one holding only `pulse/native`; then one holding neither; a unit on the host needing `audio` |
| **Action** | launch the unit under each |
| **Expected** | first `PIPEWIRE_REMOTE` and `PULSE_SERVER=unix:<path>`, each an absolute path; then `PULSE_SERVER` alone; then `Failed`, its failure naming audio |

## yoke:what-a-need-expands-into.05 — in a container, each need is mounted or mapped at the same path, and a network given only where asked

| Field | Value |
| --- | --- |
| **Cites** | specs/45.11 · specs/45.12 · specs/45.16 · specs/45.17 · arch/35-units/05 §What each declared need expands into · arch/35-units/05 §No network by default |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a containerised interface needing a device, `storage:cache`, `display`, `audio` and `network`, with the Core's environment naming a Wayland socket, an X11 display and both audio sockets; and a containerised oneshot needing nothing |
| **Action** | launch both |
| **Expected** | the first is created with the device mapped at its path and its owning group's number, the storage directory, the Wayland socket, the X11 socket, the authority file and both audio sockets mounted at their paths, a network, and `WAYLAND_DISPLAY` absolute with no `XDG_RUNTIME_DIR`; the storage directory existed before the container was created; the second is created with nothing mounted but the instance's directory, no device, no group and no network |

## yoke:what-a-need-expands-into.06 — under the environment's engine, a unit writes its storage, opens its device and reaches a network only where it asked for one

| Field | Value |
| --- | --- |
| **Cites** | specs/45.12 · specs/45.16 · specs/45.17 · arch/35-units/05 §What each declared need expands into · arch/35-units/05 §No network by default |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the environment's engine serving its API; a composition with two containerised oneshots, each needing `storage:data` and `device:sink` bound to `/dev/null`, the second also needing `network` |
| **Action** | start the Core and let both complete |
| **Expected** | each writes a file into its storage directory, which the host then finds under `<state>/storage/data` owned by the launching account, and opens the device for writing; the first sees only the loopback interface, the second at least one other |

## yoke:what-a-need-expands-into.07 — a device's group is carried only where the account reaches the node through it

| Field | Value |
| --- | --- |
| **Cites** | specs/45.11 · arch/35-units/05 §What each declared need expands into · arch/35-units/05 §Identity across the boundary |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three containerised units, each needing one device: a node of the account's group that only its owner and group may use; a node any account may read and write; and `/dev/null` |
| **Action** | launch them |
| **Expected** | the first is created with the node's group; the second and the third with none, every account already reaching them |
