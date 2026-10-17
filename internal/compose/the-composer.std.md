# The composer: init, check and build

| | |
| --- | --- |
| **Feature** | the three verbs that make a project and a package from it: `init` writes a descriptor with the five identity fields and the directories beside it; `check` runs the one gate over the project, composing; `build` refuses on any refusal, assembles the bundle from the descriptor, `manifests/` and `components/`, archives it deterministically, fills the footer from the descriptor's head, seals and signs where it is given a credential and a key, and names the file by identity and version — running nothing the project contains |
| **Planning item** | yoke-project/yoke#223 |

## yoke:the-composer.01 — `init` writes a descriptor with the five identity fields, and the directories beside it

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/20-composer/02 §The working directory |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty directory; then the same directory again; then another, given `Station` as the identifier |
| **Action** | `init com.yoke.station` in the first, then again in the same directory, then `init Station` |
| **Expected** | the first writes `yoke.yaml` stating `model`, `id`, `version`, `arch` and `core`, with `manifests/` and `components/` beside it; the second is refused, naming the descriptor already there, and changes nothing; the third is refused, naming the identifier |

## yoke:the-composer.02 — `check` runs the gate over the project, composing, and reports every finding

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/15-gate/06 §The moments |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project whose descriptor carries two refusals, and one that carries none |
| **Action** | `check` each |
| **Expected** | the first reports both refusals with their codes and locations and says it refused; the second refuses nothing, and the host facts are named as not run |

## yoke:the-composer.03 — `build` refuses on any refusal, and writes nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/13.30 · arch/20-composer/02 §Building a package |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project whose descriptor carries a refusal |
| **Action** | `build` it |
| **Expected** | it is refused with the gate's report, and no package is written |

## yoke:the-composer.04 — `build` refuses a path outside the bundle, and a file that is not there

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/02 §Building a package · arch/20-composer/02 §The working directory |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | projects whose unit names `src/acquire`, `../acquire`, `/usr/bin/acquire`, and `components/absent` |
| **Action** | `build` each |
| **Expected** | each is refused, naming the unit and the path; no package is written |

## yoke:the-composer.05 — two builds of one tree produce one digest

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/02 §Building a package |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project built once; its files then touched to a later modification time and the build run again |
| **Action** | compare the two packages' `digest`, and read the archive's entries |
| **Expected** | the digests are equal; the entries are in a fixed order, each dated at one constant, and carry only whether a file is executable |

## yoke:the-composer.06 — the package is the bundle and nothing else, its footer filled from the descriptor

| Field | Value |
| --- | --- |
| **Cites** | specs/13.30 · specs/14.40 · arch/20-composer/02 §Building a package · arch/20-composer/02 §The two sidecars |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project of `com.yoke.station` `2.1.0` for `aarch64`, with `advisories.yaml`, `blocks.yaml` and `src/` beside the bundle's three entries |
| **Action** | `build` it, and read the package |
| **Expected** | it is named `com.yoke.station-2.1.0.yoke`; its footer carries the model, the identifier, the version, the architecture and the Core range the descriptor states, unsigned and unsealed; its payload holds `yoke.yaml`, `manifests/` and `components/`, and neither sidecar nor `src/` |

## yoke:the-composer.07 — `build` signs with the key it is given, and seals with the credential it is given

| Field | Value |
| --- | --- |
| **Cites** | specs/13.30 · specs/13.14 · arch/20-composer/01 §One artifact, beside the manager and never on the device |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an Ed25519 key in PEM and a credential of 32 bytes in base64 |
| **Action** | `build` with the key, then with the key and the credential |
| **Expected** | the first package's signature verifies against the key; the second is sealed and opens with the credential to the same payload digest |

## yoke:the-composer.08 — `build` runs nothing the project contains

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/02 §Building a package |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project whose component is a script that would leave a file behind were it run |
| **Action** | `build` it |
| **Expected** | the package is written and the file is not there |

## yoke:the-composer.09 — the command line takes one verb per act, and its exit status tells a refusal from a fault

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/20-composer/01 §The two faces · arch/20-composer/01 §What it reports · arch/20-composer/02 §Building a package |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty directory, an Ed25519 key in a PEM file, and a seal credential in a file of one base64 line |
| **Action** | run `yoke-compose init com.yoke.station --dir <d>`, then `check --dir <d>`, then `build --dir <d> --out <o> --sign <key> --seal <credential>` once a unit is added; then a verb it does not have; then `build` into an `--out` that cannot be written |
| **Expected** | `init` exits 0, saying what it made — the descriptor, `manifests/` and `components/`; `check` on a project with no unit exits 1, a refusal, printing the refusal's code; `build` exits 0 and prints the package's path, signed and sealed; an unknown verb exits 1, naming it and the verbs there are; the build that cannot write exits 2, a fault |

## yoke:the-composer.10 — an image's archive is packaged as any component, and one no image pins is refused

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/02 §Building a package · arch/10-manager/05 §Obtaining images |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project whose unit names an image pinned by digest, carrying its archive at `components/images/<algorithm>-<hex>.tar`; then the same project carrying a second archive no image pins |
| **Action** | `build` each |
| **Expected** | the first package's payload holds the archive under `components/images/` and nothing was fetched; the second is refused, naming the archive no image pins |
