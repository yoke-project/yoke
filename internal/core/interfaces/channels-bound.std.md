# Channels declared, and bound by address class

| | |
| --- | --- |
| **Feature** | every channel the deployment declares is bound at step 9, before any unit is launched, and binding is fatal: a channel on a local socket at `interfaces/<channel>.sock`, a channel on loopback on its port as a declared weaker configuration, and a channel on a routable address refused, since this Core carries none of what such a channel must; each channel answers in the projection it declares; and a managed interface is launched as a unit, told the address of the channel it serves |
| **Planning item** | yoke-project/yoke#140 |

## yoke:channels-bound.01 — a local channel is a socket in the tree, a loopback channel a port on the loopback address

| Field | Value |
| --- | --- |
| **Cites** | specs/00.3 · specs/00.25 · specs/12.12 · specs/14.28 · specs/70.2 · specs/70.16 · specs/70.17 · arch/70-interface-surface/01 §Where a channel is bound · arch/30-core/04 §Who binds what |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root, and a deployment declaring a channel with no address and a channel on loopback at a free port |
| **Action** | bind the deployment's channels |
| **Expected** | the first is a socket at `interfaces/<channel>.sock` with the form's mode; the second listens on the loopback address at its port and nowhere else; each channel's address is reported by its name |

## yoke:channels-bound.02 — binding is fatal, and a routable channel without its material does not bind

| Field | Value |
| --- | --- |
| **Cites** | specs/70.17 · arch/70-interface-surface/01 §Managed and attached · arch/70-interface-surface/01 §Where a channel is bound · arch/80-security/03 §Where the material comes from |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment declaring a loopback channel on a port already in use; another declaring a channel on a routable address carrying both security fields, whose certificate and key are not in the store |
| **Action** | bind each |
| **Expected** | each binding fails with an error naming the channel; the routable one names the material it could not read; nothing of either deployment is left bound |

## yoke:channels-bound.03 — each channel answers in the projection it declares

| Field | Value |
| --- | --- |
| **Cites** | specs/12.11 · specs/70.10 · specs/70.11 · arch/70-interface-surface/02 §What decides that a projection exists · arch/70-interface-surface/08 §`http+ws` — the browser projection |
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
| **Cites** | specs/00.17 · specs/12.11 · specs/28.26 · specs/70.19 · specs/70.20 · arch/70-interface-surface/01 §Managed and attached · arch/35-units/04 §What the Core hands the process |
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
| **Cites** | specs/00.16 · specs/28.6 · specs/70.2 · specs/70.20 · arch/70-interface-surface/01 §Managed and attached · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a local channel served by a managed interface — a program that connects to the address in `YOKE_SOCKET` and says so — and an attached channel on loopback |
| **Action** | start the Core |
| **Expected** | the Core reports the loopback channel as a weaker configuration, binds both channels before it launches the interface, and the interface reports having connected to its channel's socket |

## yoke:channels-bound.06 — a routable channel speaks TLS 1.3 alone, with its projection's ALPN, presenting the chain from the store

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/03 §TLS 1.3 and nothing below it · arch/80-security/03 §Where the material comes from · arch/80-security/03 §What the two projections look like on a routable address · arch/80-security/03 §The caller's credential sits above the transport |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a certificate and key in the store's `tls/` for two routable channels on `127.0.0.1`, one carrying `local` and one `http+ws`, both bound |
| **Action** | connect to each with TLS 1.2 at most; then with TLS 1.3, offering `h2` and `http/1.1`; then replace the material on disk and connect again |
| **Expected** | the first connections fail at the handshake; with TLS 1.3, the `local` channel negotiates `h2` and the `http+ws` channel `http/1.1`, each presents the certificate from the store, and no client certificate is asked for; after the replacement both still present the certificate they were bound with |

## yoke:channels-bound.07 — on a routable channel, a client that has established no caller is refused before anything is attached

| Field | Value |
| --- | --- |
| **Cites** | specs/12.12 · arch/70-interface-surface/03 §Who the client is · arch/70-interface-surface/04 §What an operation is refused by, and in what order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two routable channels bound with their material, one carrying `local` and one `http+ws` |
| **Action** | attach to each over TLS presenting no credential: on the first, sending first an operation that is not `authenticate`; on the second, opening `/v1/events` |
| **Expected** | each is refused `auth.required`, nothing is attached and no `channel.attached` is published |


## yoke:channels-bound.08 — through the Core, a routable channel binds over TLS with the material in the store, and without it nothing is bound

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/03 §Where the material comes from · arch/15-gate/06 §Phase 4 — host facts · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a real Core whose composition declares a routable `http+ws` channel `remote` on `127.0.0.1`; its state's `tls/` holding a certificate and key for `remote`; then another whose state holds none |
| **Action** | start each, and connect to the first's channel over TLS 1.3 |
| **Expected** | the first binds `remote`, presents the certificate from its store with ALPN `http/1.1`, and refuses `/v1/events` with `auth.required`; the second refuses its description, `channel.tls.missing` naming `remote`, and binds no channel: nothing answers on its port |

## yoke:channels-bound.09 — through the Core, a caller on a routable channel is established by the host's authentication service

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The host's answer about a person · arch/70-interface-surface/03 §Who the client is |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a real Core, `yoke-auth` beside it asking a stack of the test's own that accepts `alice` with `hunter2x`; its composition declares a routable `local` channel `panel` on `127.0.0.1`, its material in the store |
| **Action** | attach over TLS, sending first `authenticate` with `alice` and `hunter2x`; then with `alice` and `wrong` |
| **Expected** | the first is given the opening picture and the call's answer naming `alice`; the second is refused `auth.invalid` |
