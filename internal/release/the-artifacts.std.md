# The artifacts a release hands over

| | |
| --- | --- |
| **Feature** | what `yoke`'s release verb hands over as files at a programs tag: the host's artifact — the Core, the manager and the guardian with the packaging inputs — and the developer axis's — the conformance suite with a Core binary for it to drive and the composer, and the verification tool — each for Linux `amd64` and `arm64`, statically linked, and the source archive; built the same way twice to the same bytes, uploaded to the tag's release, and each named by one manifest line with its digest |
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
| **Cites** | prj_structure/85 §The release record · prj_structure/40 F4 |
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

## yoke:the-artifacts.06 — the notices travel with the programs, and are named on each file's line

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record · prj_structure/40 E3 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository with two commands, tagged `v0.1.0`, with a `NOTICE` beside its licence |
| **Action** | run the verb at that commit |
| **Expected** | each artifact holds the `NOTICE` beside the licence, and every line names `NOTICE` among its notices |

## yoke:the-artifacts.07 — a program is built from the tag, and says the version it was built at

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §Equipping a developer · prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository with two commands, tagged `v0.1.0`, with a file that is not committed beside the tagged tree |
| **Action** | run the verb at that commit, and read the build information each program carries |
| **Expected** | every program names its module at `v0.1.0`, the tag's commit as its revision, and no modification — what lies beside the tagged tree never reaches a build |

## yoke:the-artifacts.08 — an artifact carries files of the tagged tree at their own paths

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository with two commands and a committed file `packaging/unit`, tagged `v0.1.0`, the file changed beside the tag afterwards; one artifact declared holding one command and that file |
| **Action** | run the verb at that commit |
| **Expected** | each architecture's archive holds the program, the licence and `packaging/unit` at that path, with the tagged content |

## yoke:the-artifacts.09 — `yoke`'s artifacts hold what each axis carries

| Field | Value |
| --- | --- |
| **Cites** | specs/95.3 · specs/95.4 · specs/95.14 · prj_structure/95 §The built binaries · prj_structure/40 F3 · prj_structure/40 A1 · prj_structure/40 A2 · prj_structure/40 A3 · prj_structure/40 A4 · prj_structure/40 B1 · prj_structure/40 F1 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the artifacts `yoke`'s release verb declares |
| **Action** | read them against the tree |
| **Expected** | three: `yoke`, holding `yoke-core`, `yoke`, `yoke-guardian` and `yoke-auth` with `packaging/yoke.service`, `packaging/yoke-guardian.service`, `packaging/core.yaml`, `packaging/yoke.tmpfiles` and `packaging/yoke.sysusers`; `yoke-conformance`, holding the suite, `yoke-core` and `yoke-compose`; `yoke-verify`; every program and file named exists in the tree |

## yoke:the-artifacts.10 — a programs tag hands over its modules, and builds from them and the source archive with the network off first

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/40 E2 · prj_structure/97 §What a recipe pins, and what *offline* means · prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository tagged `v0.1.0` whose command imports a module it requires through a replacement inside the tree, and a source archive asked for |
| **Action** | run the verb |
| **Expected** | the upload holds `yk-0.1.0-modules.tar.gz`, holding `yk-0.1.0/vendor/modules.txt` and the required module's source beneath `yk-0.1.0/vendor/`; one line names it as `yk-modules` with its digest; and the verb built the command from the source archive and that archive, unpacked together, with no module proxy and an empty module cache, before it handed either over |
