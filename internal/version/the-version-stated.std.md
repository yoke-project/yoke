# Every program says which version it is

| | |
| --- | --- |
| **Feature** | every program a release carries — `yoke`, `yoke-core`, `yoke-guardian`, `yoke-auth`, `yoke-compose`, `yoke-conformance` — given `--version` alone prints one line, `<program> <version>`, the version Go's build information states without its `v`, and does nothing else; a build that is no release's — a pseudo-version, `+dirty`, or none — says so on the same line |
| **Planning item** | yoke-project/yoke#389 |

## yoke:the-version-stated.01 — a release's version is stated as the release's number

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | build information stating `v0.12.0` |
| **Action** | compose the line of `yoke-core` |
| **Expected** | it is `yoke-core 0.12.0`, nothing more |

## yoke:the-version-stated.02 — a build that is no release's says so

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | build information stating a pseudo-version, `v0.12.0+dirty`, `(devel)`, and none |
| **Action** | compose the line of `yoke` for each |
| **Expected** | each begins `yoke ` and the version as stated — `(devel)` where there is none — and ends saying it is no release; none is mistaken for a release's number |

## yoke:the-version-stated.03 — every program answers `--version`, and does nothing else

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries · arch/10-manager/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the six programs built from this tree, with no state directory, no Core and no display |
| **Action** | run each with `--version` alone |
| **Expected** | each exits 0 and prints exactly one line beginning with its own name; nothing is written and nothing is started |
