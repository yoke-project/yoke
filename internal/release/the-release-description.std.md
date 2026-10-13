# The release description

| | |
| --- | --- |
| **Feature** | the description of a programs release, the part `yoke`'s release verb writes: what was merged since the previous programs tag, each pull request with its number and the issues it closes, grouped by the type of those issues, and the comparison between the two tags; rewritten only while it is as the verb left it, and naming no private repository |
| **Planning item** | yoke-project/yoke#127 |

## yoke:the-release-description.01 — what was merged is grouped by the type of the issues each pull request closes

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | five pull requests merged since `v0.4.0`: one closing a `Task`, one closing a `Bug` and a `Task`, one closing nothing, one closing a `Feature` in another public repository, and one closing a `Task` in a private repository |
| **Action** | describe `v0.5.0` from them |
| **Expected** | *Changes* lists the first, the fourth naming `owner/name#n`, and the fifth naming no issue; *Fixes* lists the second; *Not tied to an issue* lists the third; each line carries its pull request's number; the comparison links `v0.4.0...v0.5.0`; nothing names the private repository |

## yoke:the-release-description.02 — the previous programs tag is the greatest lower one, and the first release has none

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a tree tagged `v0.1.0`, `v0.2.0`, `proto/v0.9.0` and `v0.3.0`, and a forge that answers what was merged between two tags |
| **Action** | describe `v0.3.0`; then describe `v0.1.0` |
| **Expected** | the forge is asked what was merged between `v0.2.0` and `v0.3.0`, and the description compares the two; for `v0.1.0` it is asked from no previous tag, and the description carries no comparison |

## yoke:the-release-description.03 — a description is rewritten only while it is as the verb left it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release of `v0.3.0` described as the verb writes it, and then: followed by a section another party wrote; with its part edited by hand; and written by hand with no mark |
| **Action** | describe `v0.3.0` again in each case, the forge now answering one pull request more |
| **Expected** | where only another section follows, the verb's part is replaced and the section kept; where the part was edited, and where it was written by hand, the description is left as it is and the run says so; describing an unchanged release again writes nothing |

## yoke:the-release-description.04 — a programs tag's run describes its release once its files are handed over

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The acts · prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository with two commands, tagged `v0.1.0` and `proto/v0.1.0`, and a forge whose description is empty |
| **Action** | run the verb with the forge |
| **Expected** | it exits 0; the release of `v0.1.0` is described after the files were uploaded to it, and the description of the definitions tag is not asked for |
