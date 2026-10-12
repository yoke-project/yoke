# Removing, and the one reclaiming pass

| | |
| --- | --- |
| **Feature** | removing bundles — refused while any record points at a version being removed, each version marked withdrawn before anything is deleted — and the one reclaiming pass: superseded versions nothing points at, incomplete installs, Yoke's images no bundle references any more, an ephemeral record a failure left, and the runtime directory of an instance whose claim is free; never a pre-release |
| **Planning item** | yoke-project/yoke#227 |

## yoke:the-removal.01 — a removal is refused while a record points at a version being removed

| Field | Value |
| --- | --- |
| **Cites** | specs/15.55 · arch/10-manager/06 §Removing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `2.0.0` and `2.1.0` installed; an instance pinned to `2.0.0` |
| **Action** | remove the application; then remove `--version 2.1.0` |
| **Expected** | the first is refused, naming the instance and `2.0.0`, and both versions are untouched; the second removes `2.1.0` and leaves `2.0.0` |

## yoke:the-removal.02 — a removal writes withdrawn before it deletes

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/06 §Removing · arch/10-manager/04 §The install record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a version nothing points at, and a removal stopped after its first act |
| **Action** | remove it |
| **Expected** | the version's record says `withdrawn` before its directory goes, so a pin cannot enter between the two; once removed, its directory and, being the last, the application's are gone |

## yoke:the-removal.03 — the reclaiming pass collects superseded versions and incomplete installs, and never a pre-release

| Field | Value |
| --- | --- |
| **Cites** | specs/15.56 · specs/13.29 · arch/10-manager/06 §Reclaiming |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `1.0.0`, `2.0.0` and `2.1.0` complete, `2.2.0-rc.1` complete, `2.3.0` incomplete; an instance pinned to `1.0.0` |
| **Action** | run the pass over the application |
| **Expected** | `2.0.0` and `2.3.0` are removed and named; `1.0.0` is kept because an instance points at it, `2.1.0` because nothing supersedes it, and `2.2.0-rc.1` because it is a pre-release |

## yoke:the-removal.04 — the pass collects Yoke's images no bundle references, and only those

| Field | Value |
| --- | --- |
| **Cites** | specs/15.56 · arch/10-manager/06 §Reclaiming |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding images tagged `yoke/com.yoke.station:2.0.0` and `yoke/com.yoke.station:2.1.0` and one image of the account's own; `2.0.0` removed by the pass, `2.1.0` kept |
| **Action** | run the pass |
| **Expected** | the tag `yoke/com.yoke.station:2.0.0` goes and then its image; the image of `2.1.0` and the account's own are left; the engine is asked for nothing where no image is tagged |

## yoke:the-removal.05 — the pass clears a failed ephemeral record and a free runtime directory

| Field | Value |
| --- | --- |
| **Cites** | specs/15.56 · specs/17.28 · arch/10-manager/06 §Reclaiming · arch/30-core/03 §The eleven steps |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an ephemeral record whose run failed; a declared record; a runtime directory whose claim nobody holds, and one whose claim another process holds |
| **Action** | run the pass |
| **Expected** | the failed ephemeral record and its state go, and the free runtime directory; the declared record and the held directory stay |

## yoke:the-removal.06 — the pass removes Yoke's images from the engine's store, and nothing of the account's

| Field | Value |
| --- | --- |
| **Cites** | specs/15.56 · arch/10-manager/06 §Reclaiming · arch/10-manager/05 §Obtaining images |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | three OCI archives written by the test and loaded into the engine; the first tagged `yoke/com.yoke.station:2.0.0`, the second `yoke/com.yoke.station:2.1.0`, the third left under its own name; `2.0.0` and `2.1.0` installed, nothing pointing at either |
| **Action** | run the pass with the engine |
| **Expected** | `2.0.0` and its tag are gone and the pass names both; the tag of `2.1.0` and the third image are still in the store. Not applicable under Docker, whose classic store loads an archive with no digest |
