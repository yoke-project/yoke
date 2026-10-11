# A family released as its source

| | |
| --- | --- |
| **Feature** | the release verb for a family consumed as a source release — C and C++: at the family's release tag it archives the tagged tree, hands the archive to the tag's release, and emits one line naming it with its digest; no Go module is published and a definitions tag is not this family's |
| **Planning item** | yoke-project/yoke-sdk-c#26 |

## yoke:the-source-release.01 — a family's release tag hands over its source archive, and one line names it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §What C and C++ are consumed as · prj_structure/85 §What a release is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a C family's checkout, with `LICENSE` and `NOTICE`, tagged `v0.5.0` |
| **Action** | run the verb as that family's source release |
| **Expected** | one archive, `yoke-sdk-c-0.5.0-source.tar.gz`, is handed to the release of `v0.5.0`, its entries under `yoke-sdk-c-0.5.0/`; one line names `yoke-sdk-c-source` at `v0.5.0`, the commit, the archive's digest, the release as where and its origin as what authenticates it, the licence and the notice |

## yoke:the-source-release.02 — no Go module is published, and a definitions tag is not the family's

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §What C and C++ are consumed as |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same checkout, holding a `go.mod`, tagged `v0.5.0` and `proto/v0.5.0` |
| **Action** | run the verb |
| **Expected** | the module proxy is never asked for anything, and only the one archive and its one line come of it |

## yoke:the-source-release.03 — made twice, the archive is the same bytes

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §What C and C++ are consumed as · prj_structure/85 §What a release is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same tagged checkout |
| **Action** | run the verb twice |
| **Expected** | the two archives handed over are byte for byte the same |
