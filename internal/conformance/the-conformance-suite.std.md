# The conformance suite

| | |
| --- | --- |
| **Feature** | `yoke-conformance`, one program built from the definitions that drives a real Core and never a fixture: a run describes, composes, starts, drives and reports, in a temporary tree; harnesses are reached over a line-oriented control protocol on a Unix socket; the table holds pass, fail or absent per case and language; the exit status reads the table; and the run says what ran |
| **Planning item** | yoke-project/yoke#17 |

## yoke:the-conformance-suite.01 — a run describes, composes, starts, drives and reports, in a tree it removes

| Field | Value |
| --- | --- |
| **Cites** | specs/90.17 · specs/90.44 · arch/90-sdks/03 §The shape of a run · arch/90-sdks/03 §A real Core, and no fixture |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built; a harness that describes itself with a Manifest and, launched by the Core, says hello with its unit identity; one case requiring that hello |
| **Action** | run the suite |
| **Expected** | the case passes; while it ran, the tree held `core.yaml`, a composition naming the harness as a unit with `CONFORMANCE_SOCKET` in its environment, the harness's Manifest at `<plugins>/<id>/manifest.yaml` and its executable under the plugin's identity; afterwards the Core has stopped and the tree is gone |

## yoke:the-conformance-suite.02 — the Core's readiness is told, by its ready line

| Field | Value |
| --- | --- |
| **Cites** | specs/90.44 · arch/90-sdks/03 §The shape of a run |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | in place of the Core, a program that writes a line and never says it is ready |
| **Action** | run the suite with a bounded wait |
| **Expected** | the run ends without driving a case, says the Core did not become ready and what it said, and exits non-zero |

## yoke:the-conformance-suite.03 — the control protocol: one JSON object per line, hello first

| Field | Value |
| --- | --- |
| **Cites** | specs/90.17 · arch/90-sdks/04 §The control protocol · arch/90-sdks/04 §How it is reached |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the harness of case 1, and a case that issues a directive to which the harness answers with an observation and then its result |
| **Action** | run the suite |
| **Expected** | each harness's first line was a hello carrying the contract, the language, the SDK line, the declared contract version and, for the one the Core launched, its unit; the result carried the directive's identifier; the observation reached the case before the result; a finish ended both harnesses |

## yoke:the-conformance-suite.04 — an unrecognised verb makes a case absent, and a refusal travels as its code

| Field | Value |
| --- | --- |
| **Cites** | specs/90.25 · arch/90-sdks/04 §The vocabulary · arch/90-sdks/04 §The control protocol |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a case issuing a verb the harness does not recognise; a case issuing a directive the harness answers with a refusal, requiring its code |
| **Action** | run the suite |
| **Expected** | the first is absent and counts as a failure; the second passes on the code alone |

## yoke:the-conformance-suite.05 — the table: one row per case, one column per language, three values

| Field | Value |
| --- | --- |
| **Cites** | specs/90.19 · specs/90.25 · arch/90-sdks/03 §What a run produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three cases: one the harness satisfies, one it does not, one it cannot recognise |
| **Action** | run the suite |
| **Expected** | the table has a row per case under the harness's language, reading pass, fail and absent; the failing row reports the directive issued, what was required and what the harness observed; the run prints each case's result as the lines the record writer reads, `pass  <id>` and `FAIL  <id> — <what failed>` |

## yoke:the-conformance-suite.06 — the exit status is zero only when every cell passes

| Field | Value |
| --- | --- |
| **Cites** | specs/90.19 · specs/90.20 · arch/90-sdks/03 §What a run produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the suite's entry point, given a run whose cases all pass and one with a case absent |
| **Action** | run each |
| **Expected** | the first exits zero and the second non-zero |

## yoke:the-conformance-suite.07 — a run says what ran

| Field | Value |
| --- | --- |
| **Cites** | arch/90-sdks/03 §What a run produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the run of case 1 |
| **Action** | read its report |
| **Expected** | it names the Core that ran and its version, and each harness's contract, declared contract version, language and SDK line |
