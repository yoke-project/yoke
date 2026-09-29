<!-- Rendered by yoke-conformance from its cases: change the cases, not this file. -->
# The administrative contract, at L2

| | |
| --- | --- |
| **Feature** | the administrative contract, as the conformance suite measures it against a real Core through a family's harness |
| **Planning item** | yoke-project/yoke#97 |

## yoke:administrative.01 — the library reaches the instance by its addresses, and reads it

| Field | Value |
| --- | --- |
| **Cites** | specs/60.7 · specs/60.36 · specs/90.4 · arch/60-administrative-surface/01 §One pair per instance · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core started by the suite in the service form, and the harness launched by the suite with the instance's root in `CONFORMANCE_INSTANCE` |
| **Action** | `read` of the kind `instance` |
| **Expected** | one record, the instance's, ready and in the service form |

## yoke:administrative.02 — a change answers what it replaced, and an effect already true succeeds

| Field | Value |
| --- | --- |
| **Cites** | specs/60.31 · specs/60.34 · arch/60-administrative-surface/04 §What every answer carries |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the fixture plugin declared, and enabled |
| **Action** | `disable` of the fixture, twice |
| **Expected** | the first answers that it was enabled, effective immediately; the second that it was not, effective immediately |

## yoke:administrative.03 — a refusal travels as its code, with what it names

| Field | Value |
| --- | --- |
| **Cites** | specs/60.57 · specs/90.23 · arch/60-administrative-surface/07 §This surface's codes · arch/90-sdks/05 §The observable model may not differ |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit nobody declared, and a capability the fixture's Manifest does not declare |
| **Action** | `stop-unit` of `nobody`; `grant` of `head.move` to the fixture |
| **Expected** | `subject.unknown` naming the kind `unit` and the identity `nobody`; `capability.undeclared` naming `head.move` |

## yoke:administrative.04 — a grant takes effect at the next admission

| Field | Value |
| --- | --- |
| **Cites** | specs/60.33 · arch/60-administrative-surface/04 §What every answer carries |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the fixture, granted nothing |
| **Action** | `grant` of `stream.data.publish` to the fixture |
| **Expected** | that it was not granted, effective at the next admission |

## yoke:administrative.05 — a subscription opens with a snapshot, and continues with what happens

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.49 · specs/90.32 · arch/60-administrative-surface/06 §What a subscription promises |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the fixture, disabled |
| **Action** | `subscribe` to the subject kind `plugin`; then `enable` of the fixture |
| **Expected** | a snapshot observed holding the fixture's record, then an event `plugin.policy.changed` about the fixture |

## yoke:administrative.06 — the log is queried

| Field | Value |
| --- | --- |
| **Cites** | specs/60.43 · arch/60-administrative-surface/05 §The log store, queried and followed |
| **Level** | L2 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Core, ready |
| **Action** | `query-log`, from the beginning |
| **Expected** | entries, among them `instance.ready` |
