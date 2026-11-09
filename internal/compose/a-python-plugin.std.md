# A Python Plugin, as an image

| | |
| --- | --- |
| **Feature** | a Plugin project in Python, whose build leaves an OCI image archive, taken into an application as a containerised unit |
| **Planning item** | yoke-project/yoke#421 |

## yoke:a-python-plugin.01 — a Python Plugin is taken as an image pinned by its manifest's digest

| Field | Value |
| --- | --- |
| **Cites** | specs/95.45 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Python Plugin project whose build leaves an OCI archive |
| **Action** | `add plugin` of it, then build the application |
| **Expected** | the archive is at `components/images/<algorithm>-<hex>.tar`, the unit names the image pinned by the manifest's digest and no `exec`, the record holds that digest, the Manifest is placed, and the application builds |

## yoke:a-python-plugin.02 — a build that leaves no image archive is refused, and nothing is placed

| Field | Value |
| --- | --- |
| **Cites** | specs/95.45 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Python Plugin project whose build leaves a file that is no archive |
| **Action** | `add plugin` of it |
| **Expected** | it is refused, naming OCI, and the project is as it was |

## yoke:a-python-plugin.03 — taking it again replaces the image the first taking placed

| Field | Value |
| --- | --- |
| **Cites** | specs/95.42 · specs/95.45 · arch/20-composer/07 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Python Plugin taken from a path, whose build now leaves another image |
| **Action** | `update plugin` without a tag, then build the application |
| **Expected** | the first archive is gone, the second is placed, the unit's image names its digest, and the application builds |
