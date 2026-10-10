# What a unit arrives with

| | |
| --- | --- |
| **Feature** | everything a unit is given before it speaks, and nothing else: the reserved variables with every path derived from the instance root, the declaration's own environment, and a startup window that is the unit's own, the deployment's value its default |
| **Planning item** | yoke-project/yoke#14 |

## yoke:what-a-unit-arrives-with.01 — the variables and nothing else, every path derived from the root

| Field | Value |
| --- | --- |
| **Cites** | specs/28.25 · specs/28.27 · specs/40.22 · specs/50.3 · specs/50.4 · specs/50.5 · arch/50-plugin-surface/01 §What the launch supplies · arch/50-plugin-surface/01 §What it is not given |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the Core's own environment carrying a variable of its own; two Plugin units, `first` and `second`, each declaring one variable, launched by one supervisor |
| **Action** | each writes out its whole environment |
| **Expected** | each environment is exactly `YOKE_PLUGIN`, `YOKE_UNIT`, `YOKE_SOCKET`, `YOKE_BIND`, `YOKE_TOKEN` and its declared variables; the Core's variable is absent; `YOKE_SOCKET` and `YOKE_BIND` are under the instance root; no value names the other unit, an incarnation or a Session |

## yoke:what-a-unit-arrives-with.02 — the startup window is the unit's, and the deployment's is its default

| Field | Value |
| --- | --- |
| **Cites** | specs/50.7 · specs/50.8 · arch/50-plugin-surface/01 §The startup window, and its three uses · arch/30-core/05 §Where the numbers come from |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a supervisor whose policy's startup window is five seconds; two Plugin units that never register, `quick` with a policy of its own whose window is 300 ms, and `patient` with none |
| **Action** | launch both, and look after one second |
| **Expected** | `quick` has ended `Failed`; `patient` is still `Starting` |

## yoke:what-a-unit-arrives-with.03 — a unit that runs to completion is told its identity and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/28.26 · arch/35-units/04 §What the Core hands the process |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a supervisor whose token issuer records whom it issues for; a unit of kind `oneshot` declaring one variable |
| **Action** | launch it; it writes out the reserved variables and its declared one |
| **Expected** | it arrives with `YOKE_UNIT` and its declared variable; `YOKE_PLUGIN`, `YOKE_SOCKET`, `YOKE_BIND` and `YOKE_TOKEN` are absent; no bootstrap token was issued, and its status carries none |
