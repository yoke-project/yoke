# A command's payload

| | |
| --- | --- |
| **Feature** | what the Core keeps of a command's payload: nothing. A payload is carried to the unit's Session and read by nobody in between, so none — carried, refused before any Session sees it, or given on the administrative surface — is written to the Core's output, at any level of the log, or to any file under the state directory, where the log store keeps the events and the units' lines |
| **Planning item** | yoke-project/yoke#462 |

## yoke:a-command-payload.01 — through the Core, a secret in a command's payload is written nowhere

| Field | Value |
| --- | --- |
| **Cites** | specs/31.23 · specs/31.54 · arch/70-interface-surface/04 §What is opaque, and what that costs the Core · arch/45-events/06 §The catalogue |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and logging at `debug`, with a composition declaring a local channel and a Plugin unit of a program that registers, opens its Session and acknowledges a command as done; on the operator projection, the command's capability granted and the unit restarted |
| **Action** | attach to the channel and command the unit with a random secret as the payload; command a type nobody declared with a second; give the command on the administrative surface with a third; wait until `unit.command.issued` is found under the state directory |
| **Expected** | the first and third are acknowledged as done and the second refused; none of the three secrets is in the Core's output or in any file under the state directory |
