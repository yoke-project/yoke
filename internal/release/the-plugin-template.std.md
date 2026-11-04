# A family's Plugin template, handed over

| | |
| --- | --- |
| **Feature** | the release verb, given `-template <family>`, hands over with a family's release the archive of the tagged tree's `template/` directory, `yoke-plugin-template-<family>-<version>.tar.gz`, its entries under `template/`, made the same way twice, to the tag's release beside whatever else the family publishes, and emits one line naming `yoke-plugin-template-<family>` with its digest; a tree with no `template/` is refused, and nothing is handed over |
| **Planning item** | yoke-project/yoke-sdk-go#76 |

## yoke:the-plugin-template.01 — a family's release tag hands over its template, and one line names it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §A family's Plugin template · arch/20-composer/07 §The template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a C family's checkout holding `template/yoke-plugin.yaml` and `template/plugin/main.c`, tagged `v0.5.0` |
| **Action** | run the verb as that family's source release, with its template |
| **Expected** | two archives are handed to the release of `v0.5.0`: the source, and `yoke-plugin-template-c-0.5.0.tar.gz`, whose entries are exactly `template/yoke-plugin.yaml` and `template/plugin/main.c` with the tree's bytes; two lines, the second naming `yoke-plugin-template-c` at `v0.5.0`, the commit, the template archive's digest, the release as where and its origin as what authenticates it |

## yoke:the-plugin-template.02 — a tree with no template is refused, and nothing is handed over

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §A family's Plugin template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same checkout without `template/`, tagged `v0.5.0` |
| **Action** | run the verb with the family's template |
| **Expected** | it exits non-zero naming `template/`; nothing is uploaded and no line is written |

## yoke:the-plugin-template.03 — made twice, the template's archive is the same bytes

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §A family's Plugin template · arch/20-composer/07 §The template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the checkout of `.01` |
| **Action** | run the verb twice |
| **Expected** | the two template archives are byte for byte the same, and so are their digests |
