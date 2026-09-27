# The definitions packages' publication

| | |
| --- | --- |
| **Feature** | at a definitions tag, `yoke`'s `release` verb publishes the crate and the wheel, each to its registry, under a credential the release run's identity is exchanged for; a version the registry already serves is not published again, and what a registry serves is checked against the tree before a line names it — so a version published by hand, or by the other run of a commit carrying two tags, is recorded the same way, and never twice |
| **Planning item** | yoke-project/yoke#77 |

## yoke:the-packages-publication.01 — a definitions tag publishes the crate and the wheel, and emits a line for each

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §What a release is · prj_structure/85 §The release record · prj_structure/97 §The definitions, and the four families that are not Go |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository tagged `proto/v0.2.0`, with a `NOTICE`, and two registries serving nothing |
| **Action** | run the verb at that commit, on 2026-09-26 |
| **Expected** | the crate and the wheel were each published once, packaged at `0.2.0`; beside the definitions module's line, one line names `crates.io/yoke-proto` and one `pypi.org/yoke-proto`, each at `v0.2.0` with the digest of the file its registry serves, its registry's page for the version as where, the registry as the mechanism, `Apache-2.0`, `NOTICE` and `2026-09-26` |

## yoke:the-packages-publication.02 — a version already served with the tree's sources is recorded, and not published again

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record · prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `proto/v0.2.0`, and two registries already serving the crate and the wheel as the tree packages them |
| **Action** | run the verb at that commit |
| **Expected** | nothing is published; both lines are emitted, as in the first case |

## yoke:the-packages-publication.03 — a served package whose sources differ from the tree's is refused, and no line is emitted

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `proto/v0.2.0`, and a crate registry serving `0.2.0` with one source file other than the tree's |
| **Action** | run the verb at that commit |
| **Expected** | it exits non-zero, naming the registry and the file that differs; standard output holds no line |

## yoke:the-packages-publication.04 — a failed publication fails the verb, after the other was tried, and no line is emitted

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record · prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `proto/v0.2.0`, a crate registry that refuses the publication, and a wheel registry serving nothing |
| **Action** | run the verb at that commit |
| **Expected** | it exits non-zero, saying the crate was not published and why; the wheel was published all the same; standard output holds no line — so running it again, once the crate's credential exists, publishes only what is missing |

## yoke:the-packages-publication.05 — a publication refused because the version is already served is recorded

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `proto/v0.2.0`, and a crate registry that serves nothing when asked first, then refuses the publication while beginning to serve the tree's crate — the other run of the same commit having published it in between |
| **Action** | run the verb at that commit |
| **Expected** | it exits zero and the crate's line is emitted |

## yoke:the-packages-publication.06 — a programs tag alone publishes no package

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §What a release is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `v0.1.0` alone, and two registries |
| **Action** | run the verb at that commit |
| **Expected** | neither registry is asked anything, and no line names a package — the packages carry the definitions' number, which only a definitions tag moves |

## yoke:the-packages-publication.07 — the wheel is published under a credential minted from the run's identity

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · prj_structure/85 §Signing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an environment offering a workflow identity token, and a package index that names its audience, mints a credential for that token, and accepts uploads |
| **Action** | publish a wheel to it |
| **Expected** | the identity token was requested for the audience the index names; the index was given that token and minted a credential; the wheel was uploaded under that credential, with its name, version and digest — no credential was read from anywhere else |

## yoke:the-packages-publication.08 — the release run offers what trusted publishing needs

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release workflow |
| **Action** | read it |
| **Expected** | it may request an identity token; before the verb runs, a step exchanges that identity for a crates.io credential, and does not fail the run when the exchange is refused — as it is until the crate exists — and the verb is given the credential |

## yoke:the-packages-publication.09 — the packages alone can be published, by hand

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `v0.1.0` and `proto/v0.2.0`, and two registries serving nothing |
| **Action** | run the verb at that commit, asked for the packages only |
| **Expected** | the proxy is asked nothing and no file is handed over; the crate and the wheel are published, and their two lines are the only ones emitted |
