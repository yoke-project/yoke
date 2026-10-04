# What a channel sees

| | |
| --- | --- |
| **Feature** | the one record a read, a subscription's snapshot and the opening picture are made of, over the three subject kinds a channel observes — the instance, every unit, and the channel itself and no other; a read answers it, and a subscription opens with it and carries the events its filter selects among those the channel observes; a kind the channel does not observe is accepted and matches nothing; and the administrative surface reads every channel's record, which it has lacked until now |
| **Planning item** | yoke-project/yoke#143 |

## yoke:what-a-channel-sees.01 — a read answers the records of what the channel observes, and refuses what it cannot name

| Field | Value |
| --- | --- |
| **Cites** | specs/70.8 · specs/70.48 · arch/70-interface-surface/05 §What a channel observes, and what it may address · arch/70-interface-surface/05 §Three subject kinds, and three it does not see · arch/70-interface-surface/04 §The eleven |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to one of two channels, in a deployment with two units |
| **Action** | read the instance, the units, one unit by name, the channel kind, this channel by name, the other channel by name, a unit nobody declared, the plugin kind, and a kind that is none |
| **Expected** | the instance's record; both units' records; the one unit's; this channel's record alone, for the kind and by name; the other channel and the undeclared unit refused `subject.unknown`, naming the kind and the identity; no record for the plugin kind; `operation.malformed` for the kind that is none |

## yoke:what-a-channel-sees.02 — a read and the opening picture are one record

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/05 §One model, not two · arch/70-interface-surface/03 §The opening picture |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and nothing changing in the deployment |
| **Action** | read every unit, and compare with the opening picture's units |
| **Expected** | each unit's record read is the record the picture carried |

## yoke:what-a-channel-sees.03 — a subscription opens with the records its filter selects, then the events it selects that the channel observes

| Field | Value |
| --- | --- |
| **Cites** | specs/70.7 · arch/70-interface-surface/05 §What a subscription promises · arch/45-events/05 §Filtering has four independent axes |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, with two units and a plugin in the deployment |
| **Action** | subscribe with a filter on one unit; then publish an event about each unit and one about the plugin; subscribe with a filter on the plugin kind and publish another event about the plugin |
| **Expected** | the first subscription opens with that unit's record alone, at a sequence, and carries only the event about that unit; the second opens with no record and carries nothing |

## yoke:what-a-channel-sees.04 — the administrative surface reads every channel's record

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.37 · arch/60-administrative-surface/05 §Which subjects are readable · arch/70-interface-surface/05 §Three subject kinds, and three it does not see |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the administrative surface, and two declared channels of which one has a client attached |
| **Action** | read the channel kind, and one channel by name |
| **Expected** | both channels, each with its name, its projection and its address class, the first attached by its client and the second not; the one named alone |

## yoke:what-a-channel-sees.05 — through the Core, a client reads its channel, and an operator reads every channel

| Field | Value |
| --- | --- |
| **Cites** | specs/70.48 · arch/70-interface-surface/05 §What a channel observes, and what it may address · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring two local channels and a unit that runs to completion |
| **Action** | attach to one channel and read its own record and the other's; read the channel kind on the operator projection |
| **Expected** | the client reads its own record attached by its account and is refused the other's with `subject.unknown`; the operator reads both channels, the one attached by that account and the other not |
