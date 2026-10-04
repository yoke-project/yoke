# The local projection

| | |
| --- | --- |
| **Feature** | the interface surface's typed projection: one service and one bidirectional method, `Attach`, where opening the stream is attaching; the Core's first frame is the opening; every call carries an identity the caller chose, is answered in frames carrying it, and completes explicitly — with its one answer, or with a completion saying whether the caller or the Core ended it; a refusal belongs to one call and does not end the attachment; and the version, a malformed request and an operation the Core does not serve are refused in that order |
| **Planning item** | yoke-project/yoke#146 |

## yoke:the-local-projection.01 — the Core's first frame is the opening, carrying the version it speaks

| Field | Value |
| --- | --- |
| **Cites** | specs/70.9 · specs/70.11 · arch/70-interface-surface/08 §`local` — the typed projection · arch/70-interface-surface/08 §How this contract states its version |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel served in the local projection |
| **Action** | attach, and send nothing |
| **Expected** | the first frame is an opening carrying no call identity, the picture and the version 1 |

## yoke:the-local-projection.02 — a call is answered in frames carrying its identity, and a refusal does not end the attachment

| Field | Value |
| --- | --- |
| **Cites** | specs/70.12 · arch/70-interface-surface/08 §`local` — the typed projection · arch/00-system/05 §How correlation is expressed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel serving `read`, answered once, and not serving `command` |
| **Action** | send `command` as call `a`, then `read` as call `b` |
| **Expected** | call `a` is refused with `operation.unknown` in one frame carrying `a`; call `b` is answered in one frame carrying `b`; the attachment is still open |

## yoke:the-local-projection.03 — the version, a malformed request and a call already in flight are refused, in that order

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/04 §What an operation is refused by, and in what order · arch/70-interface-surface/08 §This surface's error codes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel serving `read` and a `subscribe` that streams until cancelled |
| **Action** | send a request stating version 2; one stating no operation; a `subscribe` as call `s`, and a second request as call `s` while it is in flight |
| **Expected** | the first is refused `compat.unsupported`, the second `operation.malformed`, and the second request on `s` `operation.malformed`, each naming its call; the subscription on `s` goes on |

## yoke:the-local-projection.04 — a call answered by a stream completes explicitly, and says who ended it

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/08 §`local` — the typed projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel serving a `subscribe` that sends two answers and ends, and one that streams until cancelled |
| **Action** | call the first; call the second and cancel it |
| **Expected** | the first's two answers are followed by a completion by the Core; the second, once cancelled, is followed by a completion by the caller; each frame carries its call |

## yoke:the-local-projection.05 — through the Core, a client attaches to a local channel and is given the opening

| Field | Value |
| --- | --- |
| **Cites** | specs/70.2 · specs/70.11 · arch/70-interface-surface/08 §`local` — the typed projection |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring an attached channel carrying `local` |
| **Action** | attach to the channel's socket |
| **Expected** | the first frame is the opening at version 1 |
