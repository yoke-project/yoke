# The rendered description of the suite's cases

| | |
| --- | --- |
| **Feature** | the suite renders its own cases as `<contract>.std.md`, in the form every description has, from the fields each case carries; the rendering is committed beside the suite and a check fails when it is not current; L2 is accepted in a rendering and nowhere else |
| **Planning item** | yoke-project/yoke#18 |

## yoke:the-rendered-description.01 — a contract's cases are rendered in the description's form

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The contract level · testing/30 §A case |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two cases of the plugin contract, each with an identifier, a title, its citations, a precondition, what it issues and what it requires |
| **Action** | render them, and check the rendering as any description is checked |
| **Expected** | the check accepts it; each case is a section under its identifier and title, with its citations, `L2`, `test`, `—`, `blocking`, its precondition, what it issues as the action and what it requires as the expected |

## yoke:the-rendered-description.02 — the rendering committed is current, and a stale one fails

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The contract level |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the plugin contract's committed rendering, `internal/conformance/plugin.std.md`, and the suite's cases |
| **Action** | render the cases, and compare; then compare a copy with one line changed |
| **Expected** | the rendering and the committed file are the same bytes; the changed copy is reported as not current, naming the file |

## yoke:the-rendered-description.03 — L2 is accepted in a rendering and nowhere else

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The contract level · testing/30 §What checks a description |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a rendering whose cases are at L2; the same cases written by hand, without the rendering's first line; and a rendering holding a case at L1 |
| **Action** | check each |
| **Expected** | the first is accepted; the second and third are refused, naming the case and its level |

## yoke:the-rendered-description.04 — `yoke-conformance` prints a contract's rendering

| Field | Value |
| --- | --- |
| **Cites** | testing/30 §The contract level |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-conformance` built |
| **Action** | run it with `--render plugin` |
| **Expected** | it prints the plugin contract's rendering, the committed file's bytes, and exits zero without starting a Core |
