# The release description

| | |
| --- | --- |
| **Feature** | the description of a programs release, the part `yoke`'s release verb writes: the comparison between the previous programs tag and this one, what was merged being the release command's to write after it; rewritten only while it is as the verb left it |
| **Planning item** | yoke-project/yoke#127 |

## yoke:the-release-description.01 — the verb's part is the comparison between the two tags

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a previous programs tag `v0.4.0` |
| **Action** | write the verb's part for `v0.5.0` |
| **Expected** | it is the comparison linking `v0.4.0...v0.5.0` followed by the mark, and lists nothing that was merged |

## yoke:the-release-description.02 — the previous programs tag is the greatest lower one, and the first release has none

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree tagged `v0.1.0`, `v0.2.0`, `proto/v0.9.0` and `v0.3.0` |
| **Action** | describe `v0.3.0`; then describe `v0.1.0` |
| **Expected** | the description of `v0.3.0` compares `v0.2.0` with it; `v0.1.0` has no previous tag, and the verb neither reads nor writes its description |

## yoke:the-release-description.03 — a description is rewritten only while it is as the verb left it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release of `v0.3.0` described as the verb writes it, compared with `v0.1.0`, and then: followed by a section another party wrote; with its part edited by hand; and written by hand with no mark |
| **Action** | describe `v0.3.0` again; tag `v0.2.0` and describe it again in each case |
| **Expected** | describing an unchanged release again writes nothing; where only another section follows, the verb's part is replaced by the comparison with `v0.2.0` and the section kept; where the part was edited, and where it was written by hand, the description is left as it is and the run says so |

## yoke:the-release-description.04 — a programs tag's run describes its release once its files are handed over

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts · prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository with two commands, tagged `v0.0.9` and, at the commit after, `v0.1.0` and `proto/v0.1.0`, and a forge whose description is empty |
| **Action** | run the verb with the forge |
| **Expected** | it exits 0; the release of `v0.1.0` is described after the files were uploaded to it, and the description of the definitions tag is not asked for |
