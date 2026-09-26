# The artifacts a release hands over

| | |
| --- | --- |
| **Feature** | what `yoke`'s release verb hands over as files at a programs tag: the developer axis's artifacts — the conformance suite with a Core binary for it to drive, and the verification tool — each for Linux `amd64` and `arm64`, statically linked, and the source archive; built the same way twice to the same bytes, uploaded to the tag's release, and each named by one manifest line with its digest |
| **Planning item** | yoke-project/yoke#21 |

## yoke:the-artifacts.01 — a programs tag builds each artifact for both architectures, statically linked

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §Equipping a developer · prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository tagged `v0.1.0` with two commands, and one artifact declared holding both |
| **Action** | run the verb at that commit |
| **Expected** | it uploads to `v0.1.0` one archive per architecture, `amd64` and `arm64`, each holding the two programs and the licence; each program is an ELF executable for its architecture with no interpreter — statically linked |

## yoke:the-artifacts.02 — the source archive is built from the tag and handed over beside them

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same repository, with a file that is not committed beside the tagged tree |
| **Action** | run the verb at that commit |
| **Expected** | it uploads a source archive holding every file of the tagged tree under one directory named for the release, and not the file that was never committed |

## yoke:the-artifacts.03 — each file is named by one line, with its digest

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same repository |
| **Action** | run the verb at that commit |
| **Expected** | beside the module's line, one line per uploaded file, each naming it at `v0.1.0` with the `sha256:` digest of the bytes uploaded, the tag's release as where, and the forge's HTTPS as what authenticates it |

## yoke:the-artifacts.04 — built twice, the files are the same bytes

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts · prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same repository |
| **Action** | run the verb twice at that commit |
| **Expected** | both runs upload the same files with the same digests — a recipe can pin a checksum, and a run performed again replaces nothing with something else |

## yoke:the-artifacts.05 — a definitions tag alone hands over no file, and a failed upload writes no line

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `proto/v0.2.0` alone; then tagged `v0.1.0`, with an upload that fails |
| **Action** | run the verb at each |
| **Expected** | the first uploads nothing and emits the definitions module's line alone; the second exits non-zero and emits no line |
