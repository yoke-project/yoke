# Images, as the manager obtains them

| | |
| --- | --- |
| **Feature** | what installing asks of the engine about an image, and nothing the Core asks: whether it holds an image under the name and digest a descriptor pins; a pull of that reference, reporting the engine's progress; a load of an archive, reporting what the engine said; and a tag in the reserved namespace |
| **Planning item** | yoke-project/yoke#226 |

## yoke:the-engine-images.01 — an image is present by its reference, and absent where none is held under that name and digest

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding `localhost/station@sha256:…` and nothing else |
| **Action** | ask for that reference, and for the same digest under another name |
| **Expected** | the first is present; the second is absent, and neither question changed anything |

## yoke:the-engine-images.02 — a pull asks for the reference by digest, and reports the engine's progress

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §Obtaining images · specs/15.51 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine whose pull answers three lines of progress; then one whose pull answers an error in its stream |
| **Action** | pull a reference pinned by digest from each |
| **Expected** | the first asks for exactly that reference and hands on the three lines; the second fails, naming the reference and the engine's error |

## yoke:the-engine-images.03 — an archive is loaded and what the engine said is handed on

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine that loads what it is sent, then one that refuses |
| **Action** | load an archive into each |
| **Expected** | the first is sent the archive's bytes as a tar and its line is handed on; the second fails, naming its answer |

## yoke:the-engine-images.04 — an image is tagged in the reserved namespace, by the unit that runs it

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §Obtaining images · arch/10-manager/06 §Reclaiming |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding an image |
| **Action** | tag it for the unit `pipeline` of `com.yoke.station` at `2.1.0` |
| **Expected** | the engine is asked to tag that reference as `yoke/com.yoke.station/pipeline:2.1.0` |

## yoke:the-engine-images.05 — an OCI archive named by its digest loads, and the image is present by that reference

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §Obtaining images · specs/45.42 |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | an OCI archive of one layer, written by the test and named in its index by its manifest's digest — not saved by an engine, which may re-encode the image and break the digest — and no image under that reference |
| **Action** | load the archive through the engine's API, and ask for the reference |
| **Expected** | the image is present under the name and digest the archive names. Not applicable under Docker, whose classic store loads an archive with no digest — which installing refuses as `image.undigested` |

## yoke:the-engine-images.06 — a tag in the reserved namespace is read back however the engine qualifies it

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/06 §Reclaiming · arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the tag `yoke/com.yoke.station/pipeline:2.1.0` as Docker lists it, as Podman 4 lists it (`localhost/`) and as Podman 6 lists it (`docker.io/`); the tag an earlier installation wrote, with no unit, `yoke/com.yoke.station:2.1.0`; and tags outside the namespace — another registry's `yoke/`, an image of another name, a tag with no version, a tag with an empty unit or a unit holding a slash |
| **Action** | read each back |
| **Expected** | the three spellings and the earlier tag each name `com.yoke.station` at `2.1.0`; none of the others names anything |
