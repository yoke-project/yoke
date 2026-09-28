# The event bus

| | |
| --- | --- |
| **Feature** | one in-process bus the Core's subsystems publish on and subscribe to: it numbers every event it publishes, tells every subscriber and never waits for one, bounds each subscriber's queue at 256 and announces an overflow rather than dropping in silence or closing, is reached from outside only through a surface — never through the plugin surface — and is what the Core's own record of a deployment is told from |
| **Planning item** | yoke-project/yoke#86 |

## yoke:the-event-bus.01 — every event published is numbered, and one that does not fit is not published

| Field | Value |
| --- | --- |
| **Cites** | specs/20.35 · specs/31.10 · arch/45-events/01 §The cause makes a cascade readable · arch/30-core/06 §What it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus with one subscriber |
| **Action** | publish three events, one of which names a type nobody declared |
| **Expected** | the two others are numbered 1 and 2 in the order published and reach the subscriber carrying their numbers; the third is refused where it was published, and consumes no number |

## yoke:the-event-bus.02 — a subscriber is told, in order, and publishing never waits for it

| Field | Value |
| --- | --- |
| **Cites** | specs/20.36 · specs/20.37 · specs/31.2 · specs/31.30 · arch/30-core/06 §Nothing that watches a deployment may poll · arch/45-events/04 §4 — Ordering |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus with a subscriber that reads, and one that never reads |
| **Action** | publish two hundred events about two subjects, interleaved; then ten thousand more |
| **Expected** | the reading subscriber is told the two hundred, each subject's in the order published; publishing the ten thousand returns within a second, without waiting for the subscriber that never reads |

## yoke:the-event-bus.03 — a queue holds 256, and an overflow is announced, never silent and never a close

| Field | Value |
| --- | --- |
| **Cites** | specs/20.41 · specs/31.28 · specs/31.29 · arch/30-core/06 §What the bus owes a subscriber |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bus with a subscriber that has not read yet, and another that reads |
| **Action** | publish 256 events; read one from the first; publish 300 more; then read the first until it is empty; publish one more |
| **Expected** | the first 256 fit, and the first subscriber is told them; when its queue overflows it is told so, before any event published after the overflow; its subscription stays open and is told the event published last; the reading subscriber is told all 557 |

## yoke:the-event-bus.04 — the plugin surface carries no subscription

| Field | Value |
| --- | --- |
| **Cites** | specs/20.38 · specs/20.39 · arch/30-core/06 §Subscription is a projection, never an extension |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the plugin contract's definitions |
| **Action** | read every service, method and message they define |
| **Expected** | none subscribes to events or carries one of the Core's — a unit has no view of the deployment it runs in |

## yoke:the-event-bus.05 — through the Core, a unit's life and the instance's readiness are published

| Field | Value |
| --- | --- |
| **Cites** | specs/20.37 · specs/31.43 · arch/30-core/06 §Nothing that watches a deployment may poll · arch/45-events/06 §The set |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition running one unit of a program that registers, opens its Session, heartbeats, and closes after a second |
| **Action** | start the Core |
| **Expected** | the Core's output records `instance.ready`, and `unit.state.changed` events for the unit's first incarnation into `Starting`, `Admitted` and `Running`, numbered in increasing order |
