# Subscriptions on the administrative surface

| | |
| --- | --- |
| **Feature** | `subscribe` as the bus projected outwards: a snapshot made of the records a read answers, taken at a sequence, then the events the filter selects from the next one, on the same four axes the bus filters by; a queue of 256 per subscription, and an overflow announced with a fresh snapshot; on the shell, a subscription standing from the moment a connection opens, with every subject and every type, and at most eight on one connection; and the subscriptions a connection holds, read with it |
| **Planning item** | yoke-project/yoke#96 |

## yoke:subscriptions.01 — a subscription opens with a snapshot of records at a sequence, and continues from the next

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.49 · arch/60-administrative-surface/06 §What a subscription promises · arch/60-administrative-surface/05 §One model, not two |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two units declared, and the bus having published events already |
| **Action** | subscribe by `Watch` to the subject kind `unit`; then publish a change of `acquire`'s state and one of the instance |
| **Expected** | the first answer is a snapshot at the last sequence published, holding the two units' records, the same a read of `unit` answers; the next is `acquire`'s change, numbered after the snapshot; the instance's change is not delivered |

## yoke:subscriptions.02 — a client selects on four independent axes

| Field | Value |
| --- | --- |
| **Cites** | specs/60.50 · arch/60-administrative-surface/06 §What a client selects with |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface subscribing on the bus |
| **Action** | subscribe with the floor 50 and the type prefix `unit.state`; publish a state change at 10, one at 50, and a condition change at 70 |
| **Expected** | only the state change at 50 is delivered |

## yoke:subscriptions.03 — an overflow is announced with a fresh snapshot

| Field | Value |
| --- | --- |
| **Cites** | specs/60.49 · arch/60-administrative-surface/06 §What a subscription promises |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a subscription by `Watch` whose client reads nothing |
| **Action** | publish 2000 events of 1 KiB each; then read |
| **Expected** | after the events that were delivered comes an overflow carrying a snapshot at a later sequence, and what follows it is numbered after that sequence |

## yoke:subscriptions.04 — on the shell a subscription stands from the start, and is an ordinary one

| Field | Value |
| --- | --- |
| **Cites** | specs/60.24 · specs/60.51 · arch/60-administrative-surface/06 §Who subscribes differs; what is promised does not |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface subscribing on the bus |
| **Action** | open a shell connection; publish an event; cancel the standing subscription's call; publish another |
| **Expected** | the opening names the standing subscription's call; the next frame is its snapshot, of every subject; the event arrives as an event frame carrying that call; the cancellation is answered by its completion, and the second event is not delivered |

## yoke:subscriptions.05 — a shell connection holds at most eight subscriptions, and they are read with it

| Field | Value |
| --- | --- |
| **Cites** | specs/60.39 · arch/60-administrative-surface/06 §Who subscribes differs; what is promised does not · arch/60-administrative-surface/03 §A connection as an object |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a shell connection with its standing subscription |
| **Action** | subscribe seven times more, each to the subject kind `unit`; subscribe once more; read the connections |
| **Expected** | the seven are answered with snapshots; the one more, a ninth, is refused with `operation.malformed`, naming the limit; the connection's record lists eight subscriptions, the standing one's filter empty and the seven naming `unit` |

## yoke:subscriptions.06 — through the Core, a person on the shell sees what an operator does

| Field | Value |
| --- | --- |
| **Cites** | specs/60.24 · specs/60.48 · arch/60-administrative-surface/06 §Neither projection may be asked twice |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and started in the service form, with one Manifest in its Plugin directory |
| **Action** | open a shell connection; disable the plugin by `Call` |
| **Expected** | the standing snapshot holds the instance's record, ready; then an event frame carries `plugin.policy.changed` about the plugin, with the operator as its actor |
