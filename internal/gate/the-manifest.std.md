# The Manifest

| | |
| --- | --- |
| **Feature** | the static artifact that speaks for a Plugin before it runs, checked by the gate on its own: the model version read first, every key typed by the schema, the identity agreeing with its path, the protocol, the needs, the four declared lists, the capabilities joined to them in both directions, and the four absences refused rather than ignored |
| **Planning item** | yoke-project/yoke#14 |

## yoke:the-manifest.01 — a Manifest is read into what it declares

| Field | Value |
| --- | --- |
| **Cites** | specs/42.1 · specs/42.3 · specs/42.6 · specs/40.4 · arch/50-plugin-surface/02 §The document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Manifest of `com.yoke.station.acquire` as `arch/50-plugin-surface/02` writes it — needs, two streams, a command, a query, an occurrence and four capabilities — at `/etc/yoke/plugins.d/com.yoke.station.acquire/manifest.yaml` |
| **Action** | check it |
| **Expected** | no finding; the Manifest read carries the identity, protocol 1, the need, both streams with their tolerances, the command, the query, the occurrence, and each capability with the one object it governs |

## yoke:the-manifest.02 — `manifest` is read first, and a model this host does not implement ends the read

| Field | Value |
| --- | --- |
| **Cites** | specs/42.8 · specs/40.19 · arch/50-plugin-surface/02 §The document · arch/15-gate/06 §Phase 3 — cross-document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest with `manifest: 2`, an unknown key and no `id`; one with no `manifest` |
| **Action** | check each |
| **Expected** | the first gives `plugin.manifest.model` alone, naming 2, with every later phase not run; the second gives `field.required` for `manifest` alone |

## yoke:the-manifest.03 — an unknown key is refused at any level

| Field | Value |
| --- | --- |
| **Cites** | specs/42.7 · specs/40.18 · arch/50-plugin-surface/02 §The document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest with one unknown key at the top, in a stream, in a capability, in a `governs` block, and a `startup_window` at the top |
| **Action** | check it |
| **Expected** | five `key.unknown` refusals, one at each location — a startup window is not a Manifest's to state |

## yoke:the-manifest.04 — every value is typed by the schema, and the required keys are there

| Field | Value |
| --- | --- |
| **Cites** | specs/42.4 · specs/42.6 · specs/42.9 · arch/50-plugin-surface/02 §Where it lives, and what it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest with no `protocol`; one whose `protocol` is `one`; one whose stream says `tolerates_loss: yes`; one whose `id` is `no`, read from a path naming `no` |
| **Action** | check each |
| **Expected** | `field.required`, `field.type`, `field.type`, and `field.value` — `no` is the string `no`, and not a namespaced identifier |

## yoke:the-manifest.05 — the identity in the path and in `id` agree

| Field | Value |
| --- | --- |
| **Cites** | specs/42.2 · arch/50-plugin-surface/02 §Where it lives, and what it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Manifest of `com.yoke.station.acquire` at `manifests/com.yoke.station.acquire.yaml` in a bundle, and at `/etc/yoke/plugins.d/com.yoke.station.pipeline/manifest.yaml` |
| **Action** | check each |
| **Expected** | the first passes; the second gives `plugin.manifest.id`, naming both identities |

## yoke:the-manifest.06 — a protocol the Core does not support is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/42.10 · arch/50-plugin-surface/02 §What is refused, before anything runs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest whose `protocol` is 7 |
| **Action** | check it |
| **Expected** | `plugin.protocol.unsupported`, naming 7 and the protocols this Core speaks |

## yoke:the-manifest.07 — a need is a class from the closed set, and never a particular thing

| Field | Value |
| --- | --- |
| **Cites** | specs/42.11 · specs/42.12 · specs/40.12 · arch/50-plugin-surface/02 §`needs` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest needing `device:instrument`, `secret:token` and `network`; one needing `/dev/ttyUSB0` |
| **Action** | check each |
| **Expected** | the first passes with three needs; the second gives `field.value` |

## yoke:the-manifest.08 — a stream identifier is a path component, and a capability name is not

| Field | Value |
| --- | --- |
| **Cites** | specs/42.14 · specs/50.84 · arch/50-plugin-surface/02 §The four declared lists |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | Manifests declaring, in turn, the streams `a/b`, `..` and `.`, each governed by a capability; and one whose capability is named `stream/spectra:publish` |
| **Action** | check each |
| **Expected** | `name.not_a_path_component` for each of the three streams; the last passes |

## yoke:the-manifest.09 — both tolerances default to false, as a written value

| Field | Value |
| --- | --- |
| **Cites** | specs/42.15 · specs/42.16 · specs/42.17 · arch/50-plugin-surface/02 §The four declared lists |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest with a stream that says nothing of either tolerance, and one declaring only `tolerates_reorder: true` |
| **Action** | check it |
| **Expected** | the first stream tolerates neither loss nor reorder; the second tolerates reorder and not loss |

## yoke:the-manifest.10 — capabilities and the objects they govern are joined in both directions

| Field | Value |
| --- | --- |
| **Cites** | specs/42.18 · specs/42.20 · specs/42.21 · arch/50-plugin-surface/02 §`capabilities` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest where one capability's `governs` names nothing and another's names a stream and a command; one where a capability governs a command no list declares and a query no capability governs |
| **Action** | check each |
| **Expected** | the first gives `capability.governs.count` twice, and the join is not run on it; the second gives `capability.governs.undeclared` and `capability.ungoverned` |

## yoke:the-manifest.11 — the four absences are refused, saying where each statement lives now

| Field | Value |
| --- | --- |
| **Cites** | specs/42.23 · specs/40.8 · specs/40.9 · specs/40.15 · arch/50-plugin-surface/02 §What it may not contain |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest carrying `endpoint`, `autostart` and `digest` at the top, and a `description` on a capability |
| **Action** | check it |
| **Expected** | four `manifest.field.removed` refusals, each at its field, the messages naming where the statement lives — the unit's identity, the unit in the composing document, the descriptor, the shared vocabulary |
