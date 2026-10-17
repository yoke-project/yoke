# A descriptor's head

| | |
| --- | --- |
| **Feature** | what only the descriptor has, read before its body: `model` first, ending the read where this host does not implement it; then the identity — a namespaced identifier, a semantic version, an architecture, the range of Core versions, and the optional range of versions that may have written an instance's state — handed on with the deployment; and, inside a bundle, a Plugin's Manifest found as `manifests/<plugin>.yaml` |
| **Planning item** | yoke-project/yoke#223 |

## yoke:the-descriptor-head.01 — `model` is read first, and a model this host does not implement ends the read

| Field | Value |
| --- | --- |
| **Cites** | specs/12.6 · specs/14.39 · specs/95.23 · arch/15-gate/04 §What only the descriptor has · arch/15-gate/06 §Phase 0 — reading |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | descriptors with no `model`, with `model: one`, with `model: 2` and an unknown key beside it, and with `model: 1` |
| **Action** | check each |
| **Expected** | the first is refused `field.required`, the second `field.type`, the third `model.unknown` and nothing else — no later phase runs; the last reads on |

## yoke:the-descriptor-head.02 — the identity is read and handed on with the deployment

| Field | Value |
| --- | --- |
| **Cites** | specs/12.5 · specs/13.27 · specs/14.39 · specs/14.40 · arch/15-gate/04 §What only the descriptor has |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor of `com.yoke.station`, `2.1.0`, `aarch64`, `>=2.0 <3.0`, `data: ">=2.0"`, with one unit |
| **Action** | check it |
| **Expected** | nothing is refused, and the deployment carries the model, the identifier, the version, the architecture, the Core range and the data range as written |

## yoke:the-descriptor-head.03 — each field of the identity is required, and must be what it says

| Field | Value |
| --- | --- |
| **Cites** | specs/12.5 · specs/13.27 · specs/13.28 · specs/14.39 · arch/15-gate/04 §What only the descriptor has · arch/15-gate/06 §Phase 1 — shape |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | descriptors missing `id`, `version`, `arch` and `core` in turn; and with `id: Station`, `version: 2.1`, `core: "2.0"`, `data: "~2"` |
| **Action** | check each |
| **Expected** | each missing field is refused `field.required`, naming it; `Station` is refused `field.value`, `2.1` `version.not_semver`, and the two ranges `field.value`, each naming what was written |

## yoke:the-descriptor-head.04 — inside a bundle a Plugin's Manifest is `manifests/<plugin>.yaml`

| Field | Value |
| --- | --- |
| **Cites** | specs/12.30 · specs/14.40 · specs/40.17 · arch/15-gate/04 §The third document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor whose plugin unit names `com.yoke.station.acquire`, and a `manifests/` directory holding `com.yoke.station.acquire.yaml`; then the same directory without it |
| **Action** | check the descriptor against the directory, composing |
| **Expected** | the first reads that Manifest and refuses nothing; the second is refused `plugin.manifest.missing`, naming `manifests/com.yoke.station.acquire.yaml` |

## yoke:the-descriptor-head.05 — a stamp outside the data range is refused naming the range as it is read

| Field | Value |
| --- | --- |
| **Cites** | specs/12.23 · specs/12.24 · specs/17.51 · specs/25.67 · arch/15-gate/04 §Identity · arch/15-gate/06 §Phase 3 — cross-document · arch/40-state/05 §What is compared against it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `1.0.0` declaring `data: ">=1.0"`, and state stamped `1.1.0`; `2.0.0` declaring `data: ">=1.0 <2.1"`, and state stamped `2.1.0` |
| **Action** | ask whether each may read its state |
| **Expected** | both are refused naming the stamp; the first names its range as it is read — `>=1.0` up to `1.0.0`, the declaring version being the unwritten upper end — and the second its range as written |
