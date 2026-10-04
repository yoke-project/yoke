# Attaching

| | |
| --- | --- |
| **Feature** | what attaching to a channel does: the client is established by the class of the channel's address and never by anything it states; a `single` channel refuses a second client and leaves the first untouched; the client is given the opening picture — what the channel may address, the state of each subject, the channel's own state, the standing subscription and the sequence it was taken at; the standing subscription carries what the channel observes from then on; an attachment ends with `channel.detached`, leaving nothing behind; and on a channel named in an arbitration rule the subscription must be confirmed, and goes stale when it is not |
| **Planning item** | yoke-project/yoke#141 |

## yoke:attaching.01 — the client is established by the class of the address, and channel.attached carries it

| Field | Value |
| --- | --- |
| **Cites** | specs/70.17 · arch/70-interface-surface/03 §Who the client is · arch/45-events/06 §`channel` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel on a local socket and a channel on loopback, each served |
| **Action** | attach to each |
| **Expected** | on the local socket the client is the account the kernel names for the connecting process; on loopback it is `unestablished`; each attachment publishes `channel.attached` about its channel, carrying that client; the opening's channel record says it is attached, and by whom |

## yoke:attaching.02 — a single channel refuses a second client and leaves the first untouched

| Field | Value |
| --- | --- |
| **Cites** | specs/70.42 · specs/70.43 · specs/70.44 · arch/70-interface-surface/03 §How many clients |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel declared `single`, and one declared `multiple` |
| **Action** | attach twice to each |
| **Expected** | the second attachment to the single channel is refused with `channel.in_use`, naming nothing about who holds it, and the first still answers; both attachments to the multiple channel are given the opening |

## yoke:attaching.03 — the opening picture is what the channel may address and observe, at a sequence

| Field | Value |
| --- | --- |
| **Cites** | specs/70.9 · specs/70.48 · arch/70-interface-surface/03 §The opening picture · arch/70-interface-surface/05 §What a channel observes, and what it may address |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment with a running Plugin unit granted one stream, one command and one query of those its Plugin declares, a unit that runs to completion, and two channels |
| **Action** | attach to one of the channels |
| **Expected** | the picture holds the instance, every unit with its state, and this channel's own record and not the other's; the Plugin unit's record says what may be addressed on it — the granted stream, command and query — and the other unit's says nothing may; the picture's sequence is the bus's at the moment of attaching, and the opening names the standing subscription |

## yoke:attaching.04 — the standing subscription carries what the channel observes, and nothing it does not

| Field | Value |
| --- | --- |
| **Cites** | specs/70.7 · specs/70.48 · arch/70-interface-surface/05 §What a subscription promises · arch/70-interface-surface/05 §Three subject kinds, and three it does not see |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and its standing subscription |
| **Action** | publish an event about a unit, one about the instance, one about a plugin, one about a connection, one about another channel, and one about this channel |
| **Expected** | the events about the unit, the instance and this channel arrive on the standing subscription's call, in order and after the picture's sequence; the others do not arrive |

## yoke:attaching.05 — an attachment that ends publishes channel.detached, and leaves nothing behind

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/03 §What ends an attachment · arch/45-events/06 §`channel` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a single channel |
| **Action** | the client closes its side; a client attaches again |
| **Expected** | `channel.detached` is published carrying the client and the reason `closed`; the second client is given a fresh opening, its channel record attached and the channel no longer in use by the first |

## yoke:attaching.06 — on a channel named in a rule the subscription is confirmed, and goes stale when it is not

| Field | Value |
| --- | --- |
| **Cites** | specs/70.31 · specs/70.32 · specs/70.33 · arch/70-interface-surface/03 §A connection that looks open proves nothing · arch/45-events/04 §A subscription may be confirmed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel named in an arbitration rule and one not, each attached, with a confirmation interval of 100 ms and a tolerance of three |
| **Action** | on the first, confirm the picture's sequence for half a second, then stop; on the second, never confirm |
| **Expected** | `confirm` is answered while it is sent; within half a second of the last confirmation the first channel publishes `channel.subscription.stale`, carrying when it last confirmed; the second never goes stale |

## yoke:attaching.07 — through the Core, a client attaches, is told who it is, and a second is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/70.9 · specs/70.44 · arch/70-interface-surface/03 §The opening picture |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a single channel carrying `local` and a unit that runs to completion |
| **Action** | attach to the channel's socket; attach again |
| **Expected** | the opening's picture holds the unit and the channel's record, attached by the test's own account; the Core's output records `channel.attached`; the second attachment is refused with `channel.in_use` |
