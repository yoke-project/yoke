# Asking the host

| | |
| --- | --- |
| **Feature** | how the runtime asks the host's authentication service: the program beside the Core's own executable, executed once per authentication with the account and secret on its standard input; its exit status read as accepted, refused or could not be asked; a bounded wait; and the probe a party about to execute a Core runs |
| **Planning item** | yoke-project/yoke#277 |

## yoke:asking-the-host.01 — the program beside the Core is asked, and its exit status is the answer

| Field | Value |
| --- | --- |
| **Cites** | specs/80.8 · specs/80.54 · arch/80-security/04 §The host's answer about a person |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory holding a Core's executable and, beside it, a program that records its arguments and standard input and exits with a status given to it |
| **Action** | ask for `alice` and `hunter2x`, the program exiting `0`, then `1`, then `2`, then `7` |
| **Expected** | the program is the one beside the Core, given no argument and `alice`, a zero byte and `hunter2x` on standard input; the answers are accepted, refused, could not be asked, and could not be asked |

## yoke:asking-the-host.02 — a program that is not there, or does not answer in time, could not be asked

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The host's answer about a person · arch/80-security/04 §The figures this document sets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core with no program beside it; then one beside it that never exits, asked with a wait of a tenth of a second |
| **Action** | ask each |
| **Expected** | each is could not be asked; the program that never exited has been killed when the answer comes; the wait set when none is given is thirty seconds |

## yoke:asking-the-host.03 — the probe says whether the service can be asked

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The host's answer about a person · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core with no program beside it; one whose program's `--probe` exits `2`; one whose exits `0` |
| **Action** | probe each |
| **Expected** | the first two are errors naming why; the third is none |
