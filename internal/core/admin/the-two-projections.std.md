# The administrative surface's two projections

| | |
| --- | --- |
| **Feature** | one contract carried in two shapes of exchange: every operation a member of one union, answered by one dispatcher whichever projection carried it; the method that carries an operation following from the shape of its answer, and an operation on the wrong one refused as malformed; on a held connection, calls interleaved and each answer correlated by the identity its caller chose; every call completing explicitly, a stream by a completion frame, and a cancellation naming the call it ends; and an operator call answered by a stream being a connection while it lasts |
| **Planning item** | yoke-project/yoke#93 |

## yoke:the-two-projections.01 — neither projection is privileged: both carry the same operations, alike

| Field | Value |
| --- | --- |
| **Cites** | specs/60.1 · specs/60.20 · arch/60-administrative-surface/02 §Neither is privileged, and the union is what enforces it · arch/60-administrative-surface/08 §Two services over one union |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving one operation answered once, `read`, and one answered by a stream, `subscribe` |
| **Action** | issue `read` by `Call` and on a shell connection; issue `subscribe` by `Watch` and on a shell connection |
| **Expected** | `read` is answered with the same answer on both; `subscribe` streams the same answers on both |

## yoke:the-two-projections.02 — the method follows from the shape of the answer, and the wrong one is refused as malformed

| Field | Value |
| --- | --- |
| **Cites** | specs/60.18 · arch/60-administrative-surface/08 §Two services over one union |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same surface |
| **Action** | issue `subscribe` by `Call`, `read` by `Watch`, and a request naming no operation on each projection |
| **Expected** | each is refused with `operation.malformed`, the refusal carried in the transport's status on the operator projection and in a refusal frame carrying the call's identity on the shell |

## yoke:the-two-projections.03 — calls on one held connection interleave, each answer carrying its caller's identity

| Field | Value |
| --- | --- |
| **Cites** | specs/60.23 · arch/60-administrative-surface/02 §Interleaving, correlation and completion |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving a `read` that waits until it is released when it names the unit `slow` |
| **Action** | on one shell connection, issue `read` of `slow` as the call `a`, then `read` of anything else as the call `b`; then release `slow` |
| **Expected** | `b`'s answer arrives first, carrying `b`; `a`'s arrives after the release, carrying `a` |

## yoke:the-two-projections.04 — every call completes explicitly, and a cancellation names the call it ends

| Field | Value |
| --- | --- |
| **Cites** | specs/60.23 · arch/60-administrative-surface/02 §Interleaving, correlation and completion · arch/60-administrative-surface/08 §The frame a held connection adds |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving a `subscribe` that streams two answers and then ends, and one, `log.follow`, that streams until it is cancelled |
| **Action** | on one shell connection, issue `subscribe` as `s`; then `log.follow` as `f`, and cancel `f` after its first answer |
| **Expected** | `s` gets its two answers and then a completion carrying `s`; `f` gets a completion carrying `f` after the cancellation, and the operation saw its context end; a call identity already in flight is refused with `operation.malformed` |

## yoke:the-two-projections.05 — an operator call answered by a stream is a connection while it lasts

| Field | Value |
| --- | --- |
| **Cites** | specs/60.40 · arch/60-administrative-surface/03 §What makes a connection a subject |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface serving `log.follow` streaming until cancelled, and `read` |
| **Action** | issue `read` by `Call`; then `log.follow` by `Watch`, read its first answer, and cancel it |
| **Expected** | the `Call` opened no connection; the `Watch` was listed as a connection of the projection `operator` while it lasted, announced by `connection.opened`, and its end by `connection.closed` with the reason `cancelled` |

## yoke:the-two-projections.06 — through the Core, both projections answer alike, and a shell's answers are correlated

| Field | Value |
| --- | --- |
| **Cites** | specs/60.20 · specs/60.23 · arch/60-administrative-surface/02 §Neither is privileged, and the union is what enforces it |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and started in the service form |
| **Action** | issue a request naming no operation, and `subscribe`, by `Call`; then, on one shell connection, two requests naming no operation as the calls `x` and `y` |
| **Expected** | both calls are refused with `operation.malformed` in the transport's status; the shell answers with two refusal frames of the same code, one carrying `x` and the other `y` |
