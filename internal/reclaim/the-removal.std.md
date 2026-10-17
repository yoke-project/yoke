# Removing, and the one reclaiming pass

| | |
| --- | --- |
| **Feature** | removing bundles — refused while any record points at a version being removed, each version marked withdrawn before anything is deleted — and the one reclaiming pass: superseded versions nothing points at, incomplete installs, Yoke's images no bundle references any more, an ephemeral record a failure left, and the runtime directory of an instance whose claim is free; never a pre-release |
| **Planning item** | yoke-project/yoke#227 |

## yoke:the-removal.01 — a removal is refused while a record points at a version being removed

| Field | Value |
| --- | --- |
| **Cites** | specs/15.55 · specs/17.49 · arch/10-manager/06 §Removing |
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
| **Cites** | specs/17.21 · arch/10-manager/06 §Removing · arch/10-manager/04 §The install record |
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
| **Cites** | specs/10.37 · specs/15.56 · arch/10-manager/06 §Reclaiming |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding two images of `2.0.0`, tagged `yoke/com.yoke.station/pipeline:2.0.0` and `yoke/com.yoke.station/archive:2.0.0`, one of `1.9.0` tagged as an earlier installation tagged, `yoke/com.yoke.station:1.9.0`, one of `2.1.0` tagged `yoke/com.yoke.station/pipeline:2.1.0`, and one image of the account's own; `1.9.0` and `2.0.0` removed by the pass, `2.1.0` kept |
| **Action** | run the pass |
| **Expected** | every tag of `1.9.0` and `2.0.0` goes and then its image — all three images; the image of `2.1.0` and the account's own are left; the engine is asked for nothing where no image is tagged |

## yoke:the-removal.05 — the pass clears a failed ephemeral record and a free runtime directory

| Field | Value |
| --- | --- |
| **Cites** | specs/15.56 · specs/17.28 · specs/25.48 · arch/10-manager/06 §Reclaiming · arch/30-core/03 §The eleven steps |
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
| **Precondition** | four OCI archives written by the test and loaded into the engine; the first two tagged `yoke/com.yoke.reclaim/a:2.0.0` and `yoke/com.yoke.reclaim/b:2.0.0`, the third `yoke/com.yoke.reclaim/a:2.1.0`, all through the engine's API as installing tags — where the engine qualifies the name its own way — the fourth left under its own name; `2.0.0` and `2.1.0` of `com.yoke.reclaim` installed, nothing pointing at either. The engine's store is the account's and other cases use it meanwhile, so the case names an application no other case does |
| **Action** | run the pass with the engine, over `com.yoke.reclaim` |
| **Expected** | `2.0.0` and both its tags are gone and the pass names them; the tag of `2.1.0` and the fourth image are still in the store. Not applicable under Docker, whose classic store loads an archive with no digest |
