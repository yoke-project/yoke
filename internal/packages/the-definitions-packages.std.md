# The definitions packages

| | |
| --- | --- |
| **Feature** | the definitions as a crate and a wheel, both named `yoke-proto`: their Rust and Python generated from the definitions and committed beside the Go, current with them, built, tested and packaged in container images the repository pins — so a consumer installs a package and never a protocol compiler, and the bytes a package encodes are the bytes the definitions' Go encodes |
| **Planning item** | yoke-project/yoke#77 |

## yoke:the-definitions-packages.01 — the Rust built from the definitions is committed, and current

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · prj_structure/40 A5 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions and the crate's generated Rust, as committed |
| **Action** | regenerate the Rust from the definitions |
| **Expected** | nothing changes, and no generated file is committed that the definitions do not generate — a crate is installed and never generated, so what it holds is what was committed |

## yoke:the-definitions-packages.02 — the Python built from the definitions is committed, and current

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · prj_structure/40 A5 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions and the wheel's generated Python, as committed |
| **Action** | regenerate the Python from the definitions |
| **Expected** | nothing changes, and no generated file is committed that the definitions do not generate |

## yoke:the-definitions-packages.03 — the crate builds, and encodes a message as the Go does

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · arch/00-system/05 §The encoding, and the framing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the crate, as committed, with its lockfile |
| **Action** | run its tests in the pinned Rust image |
| **Expected** | they pass: a registration request encodes to the bytes the definitions' Go encodes it to, and decodes back to itself |

## yoke:the-definitions-packages.04 — the wheel installs, and encodes a message as the Go does

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · arch/00-system/05 §The encoding, and the framing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the wheel's project, as committed |
| **Action** | build the wheel, install it into a fresh environment and run its tests, in the pinned Python image |
| **Expected** | they pass: a registration request encodes to the bytes the definitions' Go encodes it to and decodes back, and both services' stubs import |

## yoke:the-definitions-packages.05 — packaged at a version, each is named, versioned and licensed, and carries the tree's sources

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · prj_structure/85 §The series · prj_structure/40 E3 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the repository, with its licence and its notice |
| **Action** | package both at `0.1.0` |
| **Expected** | a `.crate` and a `.whl`, each named `yoke-proto` at `0.1.0`, declaring `Apache-2.0` and carrying `LICENSE` and `NOTICE`; the sources each carries are the tree's, byte for byte — the version is the definitions' number, stated when they are packaged, so the tree carries none of its own |

## yoke:the-definitions-packages.06 — every image the packages are made in is pinned

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/40 E1 · prj_structure/95 §What a contributor installs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions' script |
| **Action** | read every container image it names |
| **Expected** | at least one, and each is named by its digest — a tag moves, so an image named by a tag would generate other sources on another day, and the check that they are current would fail with nothing in the repository changed |
