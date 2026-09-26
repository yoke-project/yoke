# The release verb

| | |
| --- | --- |
| **Feature** | `yoke`'s `release` verb: run where a release's tag is seen, it publishes what the tags on this commit name into this repository's own ecosystems, and emits one line per publication on standard output in the manifest's fixed shape — what was published and its version, the commit, the digests, where and under which mechanism, the licence and the notices, the day; it reads nothing beyond its own tree |
| **Planning item** | yoke-project/yoke#20 |

## yoke:the-release-verb.01 — a programs tag publishes the module, and emits its line

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record · prj_structure/95 §The release command · prj_structure/40 G1 · prj_structure/40 G3 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository whose root module is `example.com/yk`, with a commit tagged `v0.1.0`, and a module proxy that serves the tagged tree |
| **Action** | run the verb at that commit, on 2026-09-26 |
| **Expected** | the proxy was asked for `example.com/yk@v0.1.0`; standard output is exactly one line, a JSON object naming `example.com/yk` at `v0.1.0`, the commit, the module's `h1:` digest, the proxy as where and the checksum database as the mechanism, `Apache-2.0` with no notices, and `2026-09-26` |

## yoke:the-release-verb.02 — a definitions tag publishes the definitions module, at the version the tag carries

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record · prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same repository, whose `proto/` is the module `example.com/yk/proto`, with the commit tagged both `v0.1.0` and `proto/v0.2.0` |
| **Action** | run the verb at that commit |
| **Expected** | two lines: the root module at `v0.1.0`, and `example.com/yk/proto` at `v0.2.0` with the digest of the definitions' tree alone |

## yoke:the-release-verb.03 — a proxy serving another tree is refused, and no line is emitted

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository tagged `v0.1.0`, and a module proxy serving a digest that is not the tagged tree's |
| **Action** | run the verb at that commit |
| **Expected** | it exits non-zero, saying the digest the proxy serves differs from the tree's; standard output holds no line |

## yoke:the-release-verb.04 — a commit no release tag names publishes nothing, and says so

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The release command |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository with a commit carrying no tag, and a tag that is not a release's, `nightly` |
| **Action** | run the verb at each |
| **Expected** | it exits zero, the proxy is asked nothing, standard output holds no line, and its error stream says in one line that nothing is published from this commit |
