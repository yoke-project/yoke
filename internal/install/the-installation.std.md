# Installing a package

| | |
| --- | --- |
| **Feature** | installing in its order: the footer checked against the trust store and this host before anything is written; the root checked; the version's directory made exclusively with its record `incomplete`; the payload unsealed and its digest recomputed; the bundle expanded with nothing outside it; the descriptor read and gated; every component's digest recomputed; and the record made `complete`, naming the key that verified it — executing nothing the package contains |
| **Planning item** | yoke-project/yoke#226 |

## yoke:the-installation.01 — a package is installed: its bundle under the root and its record complete

| Field | Value |
| --- | --- |
| **Cites** | specs/13.23 · specs/15.21 · specs/15.42 · specs/15.46 · specs/15.47 · arch/10-manager/05 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root, a trust store holding one key, and a package of `com.yoke.station` `2.1.0` signed with it, carrying a descriptor, a Manifest and an executable |
| **Action** | install it |
| **Expected** | `<root>/com.yoke.station/2.1.0/bundle/` holds `yoke.yaml`, the Manifest and the executable, the executable `0755` and the rest `0644`; the record is `complete`, names the key's id, says it was not sealed, carries the payload's digest and when it completed |

## yoke:the-installation.02 — a refusal before the root is touched leaves nothing behind

| Field | Value |
| --- | --- |
| **Cites** | specs/15.20 · specs/15.22 · specs/13.12 · arch/10-manager/05 §When it fails |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | packages signed by a key the store does not hold, built for another architecture, and stating a Core range this host's Core is outside |
| **Action** | install each |
| **Expected** | each is refused as itself — a failure of trust, an architecture, a Core range — and the root holds nothing |

## yoke:the-installation.03 — a failure after the directory is made leaves the version incomplete, and nothing that worked is damaged

| Field | Value |
| --- | --- |
| **Cites** | specs/15.48 · specs/15.49 · arch/10-manager/05 §When it fails |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | version `2.0.0` installed; then a package of `2.1.0` whose payload digest disagrees with its footer, and one whose descriptor the gate refuses |
| **Action** | install each |
| **Expected** | each is refused naming what failed; each `2.1.0` directory holds a record `incomplete` and no complete bundle; `2.0.0` is untouched and still complete |

## yoke:the-installation.04 — an entry that would leave the bundle is refused

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/05 §The order · specs/15.52 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | packages whose archives hold `../escape`, an absolute path, and a symbolic link |
| **Action** | install each |
| **Expected** | each is refused at expansion, naming the entry; nothing is written outside the version's directory |

## yoke:the-installation.05 — a component whose digest disagrees is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/15.17 · specs/15.18 · specs/15.24 · arch/10-manager/05 §The order · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package whose descriptor states for its executable a digest other than the file's |
| **Action** | install it |
| **Expected** | it is refused `component.digest`, naming the component; the record stays `incomplete` |

## yoke:the-installation.06 — a version already installed is reported, and nothing is done

| Field | Value |
| --- | --- |
| **Cites** | specs/15.50 · arch/10-manager/04 §Two installs at once |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same package installed once |
| **Action** | install it again |
| **Expected** | it is reported as already present, the record unchanged; nothing is written |

## yoke:the-installation.07 — a sealed package is opened with the credential, and the record says it was sealed

| Field | Value |
| --- | --- |
| **Cites** | specs/13.16 · specs/13.17 · arch/10-manager/05 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package sealed with a credential, and the credential |
| **Action** | install it without the credential, then with it |
| **Expected** | without it, it is refused at unsealing and the version is left incomplete; with it — after the incomplete version is cleared — it installs and the record says it was sealed |

## yoke:the-installation.08 — the manager reads the Core's version from its binary, and executes nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/15.23 · specs/15.33 · specs/80.41 · arch/10-manager/05 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core binary built from this tree, and a file that is not a Go program |
| **Action** | read the version of each |
| **Expected** | the first is a semantic version, read without running the binary; the second is refused, naming the file |

## yoke:the-installation.09 — every image is obtained by its digest: loaded where carried, pulled where not, and tagged

| Field | Value |
| --- | --- |
| **Cites** | specs/15.51 · specs/45.40 · arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine that holds one image already; a package whose descriptor names that image, one it carries as `components/images/<algorithm>-<hex>.tar`, and one it does not carry, each for a unit of its own, and the carried one for a second unit too |
| **Action** | install it |
| **Expected** | the held image is left as it is; the carried one is loaded once and not pulled; the other is pulled by its digest; each is tagged `yoke/com.yoke.station/<unit>:2.1.0` for every unit that runs it; what was obtained is reported per image; the record is complete |

## yoke:the-installation.10 — an archive that loads with no digest is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/45.42 · arch/10-manager/05 §Obtaining images · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine that loads an archive and then holds no image under the reference loaded, as Docker's classic store does |
| **Action** | install a package carrying that image's archive |
| **Expected** | it is refused `image.undigested`, naming the image; the record stays incomplete |

## yoke:the-installation.11 — an engine that cannot be reached is a refusal, and none is looked for where no unit names an image

| Field | Value |
| --- | --- |
| **Cites** | specs/45.38 · specs/45.39 · arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no engine at the configured address |
| **Action** | install a package whose descriptor names an image, then one whose descriptor names none |
| **Expected** | the first is refused `engine.unreachable`, naming the engine; the second installs and the engine is never asked for |

## yoke:the-installation.12 — a package the composer built, carrying an image, installs with the engine

| Field | Value |
| --- | --- |
| **Cites** | specs/15.51 · specs/45.40 · specs/13.30 · arch/10-manager/05 §Obtaining images · arch/20-composer/02 §Building a package |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | two OCI archives written by the test, each named by its manifest's digest and placed in a project's `components/images/`; no image under either reference in the engine; the project's descriptor naming one for `panel` and the other for `pipeline` |
| **Action** | build a signed package with the composer, and install it with the engine |
| **Expected** | the installation completes; the engine holds each image under its pinned reference, loaded from the package and not pulled, and each still tagged when the installation ends — `yoke/com.yoke.station/panel:2.1.0` and `yoke/com.yoke.station/pipeline:2.1.0` on the images they name. Not applicable under Docker, whose classic store loads an archive with no digest |
