# Channels declared, and bound by address class

| | |
| --- | --- |
| **Feature** | every channel the deployment declares is bound at step 9, before any unit is launched, and binding is fatal: a channel on a local socket at `interfaces/<channel>.sock`, a channel on loopback on its port as a declared weaker configuration, and a channel on a routable address refused, since this Core carries none of what such a channel must; each channel answers in the projection it declares; and a managed interface is launched as a unit, told the address of the channel it serves |
| **Planning item** | yoke-project/yoke#140 |

## yoke:channels-bound.01 — a local channel is a socket in the tree, a loopback channel a port on the loopback address

| Field | Value |
| --- | --- |
| **Cites** | specs/70.2 · specs/70.16 · specs/70.17 · arch/70-interface-surface/01 §Where a channel is bound · arch/30-core/04 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root, and a deployment declaring a channel with no address and a channel on loopback at a free port |
| **Action** | bind the deployment's channels |
| **Expected** | the first is a socket at `interfaces/<channel>.sock` with the form's mode; the second listens on the loopback address at its port and nowhere else; each channel's address is reported by its name |

## yoke:channels-bound.02 — binding is fatal, and a routable channel is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/70.17 · arch/70-interface-surface/01 §Managed and attached · arch/70-interface-surface/01 §Where a channel is bound |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment declaring a loopback channel on a port already in use; another declaring a channel on a routable address carrying both security fields |
| **Action** | bind each |
| **Expected** | each binding fails with an error naming the channel; the routable one says this Core does not carry the authenticated transport and the credential such a channel requires; nothing of either deployment is left bound |

## yoke:channels-bound.03 — each channel answers in the projection it declares

| Field | Value |
| --- | --- |
| **Cites** | specs/70.10 · specs/70.11 · arch/70-interface-surface/02 §What decides that a projection exists · arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel carrying `local` and a channel carrying `http+ws`, both bound |
| **Action** | open a gRPC connection to the first and call the interface service; send an HTTP request outside `/v1` to the second |
| **Expected** | the first answers as the interface service; the second answers `404` |

## yoke:channels-bound.04 — a managed interface is launched told the address of the channel it serves, and nothing a Plugin is told

| Field | Value |
| --- | --- |
| **Cites** | specs/70.19 · specs/70.20 · arch/70-interface-surface/01 §Managed and attached · arch/35-units/04 §What the Core hands the process |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit of kind `interface` named by a channel bound on a local socket, and a Plugin unit |
| **Action** | build each unit's environment |
| **Expected** | the interface is given `YOKE_UNIT` and `YOKE_SOCKET` carrying its channel's address, and neither `YOKE_BIND`, `YOKE_TOKEN` nor `YOKE_PLUGIN`; the Plugin is given the plugin channel in `YOKE_SOCKET`, as before |

## yoke:channels-bound.05 — through the Core, the channels are bound before the units start, and the managed interface reaches its own

| Field | Value |
| --- | --- |
| **Cites** | specs/70.2 · specs/70.20 · arch/70-interface-surface/01 §Managed and attached · arch/30-core/03 |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a local channel served by a managed interface — a program that connects to the address in `YOKE_SOCKET` and says so — and an attached channel on loopback |
| **Action** | start the Core |
| **Expected** | the Core reports the loopback channel as a weaker configuration, binds both channels before it launches the interface, and the interface reports having connected to its channel's socket |
