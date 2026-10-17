# The authentication program

| | |
| --- | --- |
| **Feature** | `yoke-auth`, the one program that asks the host's authentication service: given an account and a secret on standard input, it asks the service `yoke` and answers by its exit status alone — accepted, refused, or the service could not be asked; its probe; and that it, and nothing else of the runtime, opens a library at run time |
| **Planning item** | yoke-project/yoke#277 |

## yoke:the-authentication-program.01 — the right secret is accepted, anything else refused, and nothing is said

| Field | Value |
| --- | --- |
| **Cites** | specs/80.8 · arch/80-security/04 §The host's answer about a person |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a stack for the service `yoke` of the test's own, accepting the account `alice` with the secret `hunter2x` through a module that asks the secret by the conversation |
| **Action** | run the program with `alice`, a zero byte and `hunter2x`; then the wrong secret; then another account; then input with no zero byte, and with an empty account |
| **Expected** | the first exits `0`; every other exits `1`; nothing is written to standard output in any of them, and the secret appears in nothing written |

## yoke:the-authentication-program.02 — a service that cannot be asked is said apart from a refusal, and the probe says which

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The host's answer about a person · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory of stacks holding none for `yoke`; then the stack of `.01` |
| **Action** | run the program with `alice` and `hunter2x` against the first; run `--probe` against each |
| **Expected** | the program exits `2` against the first; the probe exits `2` against the first and `0` against the second, asking for no secret |

## yoke:the-authentication-program.03 — the Core, the manager and the guardian stay statically linked; only `yoke-auth` names an interpreter

| Field | Value |
| --- | --- |
| **Cites** | specs/17.67 · specs/80.54 · prj_structure/95 §The built binaries · arch/80-security/04 §The host's answer about a person |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core`, `yoke`, `yoke-guardian` and `yoke-auth`, each built `CGO_ENABLED=0` |
| **Action** | read each program's program headers |
| **Expected** | the first three name no interpreter; `yoke-auth` names one |
