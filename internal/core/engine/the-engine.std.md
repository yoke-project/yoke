# The engine, reached through its own API

| | |
| --- | --- |
| **Feature** | the container engine reached at the configured address through the interface it serves, and no composition tool: one projection of the engine API, at one version, for the reference engine and for the supported one; which engine it is, and whether it runs rootless, asked of the engine and never configured; an address the Core cannot speak to refused by name, and an engine that does not answer reported as unreachable, never concluded about |
| **Planning item** | yoke-project/yoke#169 |

## yoke:the-engine.01 — the engine's address is a local socket, and anything else is refused by name

| Field | Value |
| --- | --- |
| **Cites** | specs/45.37 · arch/30-core/01 §`core.yaml`, in the service form · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine answering on a local socket |
| **Action** | reach it as `unix://<path>`; then reach `tcp://127.0.0.1:2375`, `/a/bare/path` and the empty address |
| **Expected** | the first is reached; each of the others is refused before anything is dialled, the refusal naming the address |

## yoke:the-engine.02 — which engine, and whether it is rootless, is asked of the engine

| Field | Value |
| --- | --- |
| **Cites** | specs/45.31 · arch/35-units/05 §The engine, and why it is a security question |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three engines on local sockets, answering as Podman running rootless, as Docker through a root-owned daemon, and as Docker running rootless |
| **Action** | reach each |
| **Expected** | the first is `podman` and rootless, the second `docker` and not rootless, the third `docker` and rootless; each carries the version it states; nothing about either was configured |

## yoke:the-engine.03 — the Core speaks one version of the engine's API, and an engine that does not serve it is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/45.22 · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine serving versions 1.24 to 1.44 of the API, and one serving 1.44 to 1.56 |
| **Action** | reach each, recording the paths it is asked on |
| **Expected** | the first is reached and asked on `/v1.41/` alone; the second is refused, the refusal naming the range it serves and the version the Core speaks |

## yoke:the-engine.04 — an engine that does not answer is unreachable, and nothing is concluded from it

| Field | Value |
| --- | --- |
| **Cites** | specs/45.38 · arch/35-units/05 §Driving the engine · arch/30-core/05 §When the source of facts goes quiet |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a socket path where nothing listens, and a socket that accepts and never answers |
| **Action** | reach each, with a bound on the wait |
| **Expected** | each is reported as unreachable, naming the address, within the bound; neither is reported as an engine of any kind |

## yoke:the-engine.05 — the reference engine, rootless Podman, is reached and recognised

| Field | Value |
| --- | --- |
| **Cites** | specs/45.31 · arch/35-units/05 §Driving the engine · arch/35-units/05 §The engine, and why it is a security question |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | Podman, run rootless by the account running the test, serving its API on a socket in a directory of the test's |
| **Action** | reach it |
| **Expected** | it is `podman`, rootless, at the version `podman version` states |
