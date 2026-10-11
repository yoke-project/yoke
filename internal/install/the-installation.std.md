# Installing a package

| | |
| --- | --- |
| **Feature** | installing in its order: the footer checked against the trust store and this host before anything is written; the root checked; the version's directory made exclusively with its record `incomplete`; the payload unsealed and its digest recomputed; the bundle expanded with nothing outside it; the descriptor read and gated; every component's digest recomputed; and the record made `complete`, naming the key that verified it — executing nothing the package contains |
| **Planning item** | yoke-project/yoke#226 |

## yoke:the-installation.01 — a package is installed: its bundle under the root and its record complete

| Field | Value |
| --- | --- |
| **Cites** | specs/15.46 · specs/15.21 · specs/13.23 · arch/10-manager/05 §The order |
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
| **Cites** | specs/15.24 · arch/10-manager/05 §The order · arch/15-gate/06 §Phase 4 — host facts |
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
| **Cites** | specs/15.23 · specs/15.33 · arch/10-manager/05 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core binary built from this tree, and a file that is not a Go program |
| **Action** | read the version of each |
| **Expected** | the first is a semantic version, read without running the binary; the second is refused, naming the file |
