# Watching a Core

| | |
| --- | --- |
| **Feature** | the guardian watching the Cores it executed: a silence beyond the deployment's tolerance acted on in bounded steps — asked to stop, terminated after the stop window — and the instance brought back under the deployment's restart figures; an orderly shutdown the Core announced being an expected silence |
| **Planning item** | yoke-project/yoke#232 |

## yoke:watching-a-core.01 — a Core that stops saying it is alive is asked to stop, terminated, and brought back

| Field | Value |
| --- | --- |
| **Cites** | specs/17.58 · specs/17.60 · specs/17.61 · specs/17.62 · arch/25-guardian/06 §When it stops arriving · arch/25-guardian/06 §Coming back |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance whose descriptor declares a heartbeat of `1s` with a tolerance of `1`, a stop window of `2s` and a backoff of `1s`; its Core says it is alive once, then falls silent and ignores being asked to stop |
| **Action** | start the guardian; wait |
| **Expected** | once the tolerance has passed the guardian says the instance is silent, asks the Core to stop, and terminates it after the stop window; the report says the instance is being brought back, then waiting, attempt `1`; the Core is executed a second time |

## yoke:watching-a-core.02 — an orderly shutdown the Core announced is an expected silence

| Field | Value |
| --- | --- |
| **Cites** | specs/17.59 · arch/25-guardian/06 §The signal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the heartbeat of `.01`; a Core that says it is alive, then says it is stopping, stays silent longer than the tolerance, and exits zero |
| **Action** | start the guardian; wait for the Core to end |
| **Expected** | the guardian never calls the instance silent nor terminates its Core; the report says it was stopping until the Core ended |

## yoke:watching-a-core.03 — a Core built from this tree that wedges is brought back as a new incarnation

| Field | Value |
| --- | --- |
| **Cites** | specs/17.58 · specs/17.60 · specs/17.64 · arch/25-guardian/06 §When it stops arriving · arch/25-guardian/06 §Coming back |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built from this tree and named by the configuration; an instance whose descriptor declares a heartbeat of `1s`, tolerance `2`, a stop window of `2s` and a backoff of `1s`, desired `running` |
| **Action** | start the guardian; once the instance is observable, stay three intervals and check it still is; then stop the Core's process with `SIGSTOP` |
| **Expected** | while the Core runs it stays observable; once stopped, the guardian says the instance is silent, terminates the process, and executes a new Core, under another process, which becomes observable |
