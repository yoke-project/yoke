# A group's retention policy, resolved

| | |
| --- | --- |
| **Feature** | the limits the cleaner applies to each group at every cycle: the description's default, a unit's inner scope over it, and an override in the log store that replaces both, read live; an explicit zero against an absent field |
| **Planning item** | yoke-project/yoke#178 |

## yoke:a-groups-policy.01 — a unit's group takes its inner scope over the description's default, and every other group the default

| Field | Value |
| --- | --- |
| **Cites** | specs/32.5 · specs/32.6 · specs/32.8 · arch/40-state/04 §Resolving a group's policy · arch/40-state/04 §The group is the unit · arch/15-gate/03 §`policy` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment whose policy keeps 50 entries, with a unit `a` whose own policy keeps an hour and a unit `b` with none; no override in the store |
| **Action** | resolve the groups `a`, `b`, `/core` and a unit no longer declared |
| **Expected** | `a` is an hour, fifty megabytes and 50 entries; `b`, `/core` and the undeclared unit are seven days, fifty megabytes and 50 entries |

## yoke:a-groups-policy.02 — an explicit zero constrains nothing, and an absent field takes the default

| Field | Value |
| --- | --- |
| **Cites** | specs/32.1 · arch/40-state/04 §The three limits · arch/15-gate/03 §`policy` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a deployment whose policy writes `bytes: 0` and leaves age and entries out |
| **Action** | resolve a unit's group |
| **Expected** | seven days, no constraint on size, and a hundred thousand entries |

## yoke:a-groups-policy.03 — an override replaces what the description resolved to, and is read at every cycle

| Field | Value |
| --- | --- |
| **Cites** | specs/32.9 · specs/32.10 · arch/40-state/04 §Resolving a group's policy · arch/60-administrative-surface/04 §The four names, applied |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the deployment of case 01, and a resolution made before anything is written to the store |
| **Action** | write an override of 10 entries for `a`, resolve `a`; clear it, resolve `a` again |
| **Expected** | with the override, `a` keeps 10 entries and nothing constrains its age or size; once cleared, `a` is what case 01 resolved |
