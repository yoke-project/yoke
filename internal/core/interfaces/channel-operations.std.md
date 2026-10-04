# The operations, and the order a refusal is decided in

| | |
| --- | --- |
| **Feature** | the operations a channel issues against a unit — `command`, `query`, `stream.start` and `stream.stop` — each terminating in the Core and carried on that unit's Session, a command's and a question's payload opaque and bounded; `authenticate`, which on a channel whose class establishes the caller says who it is; and the order a refusal is decided in: the instance stopping, the subject, the object declared and granted, and the unit reachable |
| **Planning item** | yoke-project/yoke#142 |

## yoke:channel-operations.01 — a command is carried to the unit's Session, and its acknowledgement comes back

| Field | Value |
| --- | --- |
| **Cites** | specs/70.4 · arch/70-interface-surface/04 §The eleven · arch/70-interface-surface/04 §What is opaque, and what that costs the Core |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and a running Plugin unit whose Session acknowledges every instruction as done with a line |
| **Action** | `command` the unit with a declared and granted type and some bytes |
| **Expected** | the unit's Session is handed a command of that type carrying those bytes unchanged, and nothing naming the channel; the answer is the acknowledgement, done, with its line |

## yoke:channel-operations.02 — a question is carried, its answer comes back opaque, and both are bounded

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/04 §What is opaque, and what that costs the Core |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and a running Plugin unit whose Session answers a question with its bytes reversed |
| **Action** | `query` the unit with some bytes; then with more than 1 MiB |
| **Expected** | the answer carries the bytes reversed; the second is refused `operation.malformed` and reaches no Session |

## yoke:channel-operations.03 — a refusal is decided in the order the contract states

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/04 §What an operation is refused by, and in what order · arch/70-interface-surface/04 §What decides whether a client may issue one |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, a running Plugin unit with a Session granted one of the two command types its Plugin declares, a Plugin unit not running, one running with no Session, one that never answers, one that answers with an error, and a unit that runs to completion |
| **Action** | `command` a unit nobody declared; the unit that runs to completion; a type the Plugin never declared; the declared type not granted; the unit not running; the one with no Session; the one that never answers, with a short wait; the one that fails; then the granted type with the instance stopping |
| **Expected** | `subject.unknown` twice, naming the unit; `scope.undeclared` and `scope.withheld`, each naming the type; `unit.not_running`; `unit.no_session`; `unit.unanswered`; `unit.failed` carrying the unit's code; `instance.stopping` |

## yoke:channel-operations.04 — stream.start and stream.stop create and remove the transport around the instruction

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §Two acts, and they are not the same one · arch/70-interface-surface/04 §The eleven |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and a running Plugin unit with a Session granted a stream its Plugin declares |
| **Action** | `stream.start` the stream; then `stream.stop` it; then `stream.start` a stream not declared |
| **Expected** | the start is answered with the unit's acknowledgement after the unit was handed the activation for a transport that existed, and `unit.stream.activated` is published; the stop is answered after the unit was handed the stop, and the transport is gone; the stream not declared is refused `scope.undeclared` naming it |

## yoke:channel-operations.05 — authenticate, on a channel whose class establishes the caller, says who it is

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/04 §The eleven · arch/70-interface-surface/03 §Who the client is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel on a local socket |
| **Action** | `authenticate`, presenting nothing |
| **Expected** | the answer names the account the channel established, and carries no token |

## yoke:channel-operations.06 — through the Core, a command issued on a channel reaches the unit and its acknowledgement comes back

| Field | Value |
| --- | --- |
| **Cites** | specs/70.4 · arch/70-interface-surface/04 §The eleven |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a local channel and a Plugin unit of a program that registers, opens its Session, reports its health and acknowledges a command as done with a line |
| **Action** | on the operator projection, grant the plugin its command's capability and restart the unit; then attach to the channel and command the unit |
| **Expected** | the command is answered with the unit's acknowledgement, done, carrying its line |
