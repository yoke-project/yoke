# Versions and ranges

| | |
| --- | --- |
| **Feature** | an application's version is a semantic version, read strictly; versions are ordered as semantic versioning orders them, a pre-release below its release; a range is space-separated comparators that must all hold, a version written with fewer than three parts standing for its zeros |
| **Planning item** | yoke-project/yoke#221 |

## yoke:the-versions.01 — a version is a semantic version, or it is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/13.28 · arch/10-manager/03 §Architecture, identity and versions |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | versions written as `2.1.0`, `2.1.0-rc.1`, `2.1.0+build.7`, and as `2.1`, `v2.1.0`, `02.1.0`, `2.1.0-`, the empty string |
| **Action** | read each as an application's version |
| **Expected** | the first three are read; each of the others is refused, naming what was written |

## yoke:the-versions.02 — versions are ordered as semantic versioning orders them

| Field | Value |
| --- | --- |
| **Cites** | specs/13.29 · arch/10-manager/03 §Architecture, identity and versions |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | pairs of versions differing in each part, in a pre-release and in build metadata |
| **Action** | compare each pair |
| **Expected** | numeric parts compare as numbers, a pre-release sorts below its release and above the release before it, and build metadata does not take part |

## yoke:the-versions.03 — a range holds when every comparator holds

| Field | Value |
| --- | --- |
| **Cites** | arch/15-gate/05 §The formats · arch/15-gate/04 §What only the descriptor has |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the ranges `>=2.0 <3.0`, `>=2.0`, `=2.1.0`, `>2.0.0 <=2.1`, and versions inside, on and outside each bound |
| **Action** | ask each range whether it contains each version |
| **Expected** | a version is inside when all comparators hold; `2.0` stands for `2.0.0`; an absent upper end is unbounded |

## yoke:the-versions.04 — a range that is not one is refused

| Field | Value |
| --- | --- |
| **Cites** | arch/15-gate/05 §The formats |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the expressions `2.0`, `>=`, `>=two`, `~2.0`, two comparators joined as alternatives, the empty string |
| **Action** | read each as a range |
| **Expected** | each is refused, naming what was written |
