# A family's package

| | |
| --- | --- |
| **Feature** | `yoke`'s `release` verb, run in a family that publishes a package — asked for a crate or a wheel by its name — publishes that one package at the family's release tag, made by the family's own packaging script and published under the family's name; nothing else is published, and the checks the definitions packages pass are the same |
| **Planning item** | yoke-project/yoke#149 |

## yoke:the-family-packages.01 — a family's release tag publishes its package, made by its script, and emits one line

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/80 §The layout · prj_structure/85 §The release record · prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository with no Go module, tagged `v0.3.0`, with a `NOTICE` and a packaging script `ci/package.sh` whose `package <version> <dir>` writes the crate `yoke-sdk-<version>.crate`; and a crate registry for `yoke-sdk` serving nothing |
| **Action** | run the verb at that commit as a family publishing the crate `yoke-sdk`, on 2026-10-04 |
| **Expected** | the proxy is asked nothing and no file is handed over; the script was asked to package `0.3.0`; the crate was published once, and the one line emitted names `crates.io/yoke-sdk` at `v0.3.0`, with the digest of the file served, `Apache-2.0`, `NOTICE` and `2026-10-04` |

## yoke:the-family-packages.02 — a script that writes no package fails the verb, and no line is emitted

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository of the first case, its script refusing — as a family's does when the version it states is not the tag's |
| **Action** | run the verb at that commit as a family publishing the crate `yoke-sdk` |
| **Expected** | it exits non-zero, saying what the script said; the registry is asked nothing; standard output holds no line |

## yoke:the-family-packages.03 — each registry asks for, publishes and names the package it is given

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/80 §The layout · prj_structure/97 §The definitions, and the four families that are not Go |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a crate registry and a package index, each answering for `yoke-sdk` at `0.3.0`; the index offering a workflow identity and taking uploads |
| **Action** | ask each for `0.3.0` of the package `yoke-sdk`, and upload the wheel `yoke_sdk-0.3.0-py3-none-any.whl` to the index |
| **Expected** | the crate registry was asked for `yoke-sdk` and its file `yoke-sdk-0.3.0.crate`; each names its publication `crates.io/yoke-sdk` and `pypi.org/yoke-sdk` and its page for the version under that name; the upload names `yoke-sdk` and carries the wheel's metadata — and a registry given no name still answers for `yoke-proto` |
