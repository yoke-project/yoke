# The definitions module

| | |
| --- | --- |
| **Feature** | the definitions are a Go module of their own, nested under their directory and consumable from outside this repository with nothing else of it, published by a tag of their own at release |
| **Planning item** | yoke-project/yoke#6 |

## yoke:the-definitions-module.01 — the definitions' directory is a module, and every generated package is inside it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §Two numbers in one repository are two tag prefixes · prj_structure/97 §The definitions, and the four families that are not Go |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | read the module the definitions' directory declares, and the Go package every definition names |
| **Expected** | the directory declares `github.com/yoke-project/yoke/proto`, and every definition's Go package lies under it — a nested module is fetched at its directory's tag, so a package outside it would not be published by that tag |

## yoke:the-definitions-module.02 — the root module holds none of the definitions

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §yoke carries two, and why not one and not four |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | list the root module's packages |
| **Expected** | none lies under the definitions' directory — the programs and the definitions carry two numbers, and a package counted by both would move with either |

## yoke:the-definitions-module.03 — a consumer outside this repository builds with the module alone

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/97 §The definitions, and the four families that are not Go · prj_structure/40 E1 |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions' directory copied alone, away from the rest of this repository, and a program of another module that builds an envelope of the plugin contract, requiring the definitions module and resolving it to that copy; no network |
| **Action** | build the program |
| **Expected** | it builds — a family binds to the definitions as a package, and a package that needed anything else of this repository would be a build input nobody declared |

## yoke:the-definitions-module.04 — the verbs check both modules

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The verbs |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | read what `build`, `test` and `lint` run |
| **Expected** | each runs Go in the root module and in the definitions module — a nested module is outside the root's `./...`, and a test nobody runs is a test that never fails |
