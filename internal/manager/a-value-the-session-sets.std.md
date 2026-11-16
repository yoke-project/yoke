# A value the session sets

| | |
| --- | --- |
| **Feature** | how a value only the graphical session knows — a compositor's instance — reaches a unit: as a parameter the description declares and a unit's arguments name, set on the instance's record from that session and validated as any value is; the unit is handed it as an argument and nothing of the session's environment; the record takes it at once, a running instance included, and the Core is handed its parameters when it starts, so a value set while the instance runs reaches its units when the instance is next started |
| **Planning item** | yoke-project/yoke#461 |

## yoke:a-value-the-session-sets.01 — a parameter set from the session reaches the unit as an argument, and the session's environment does not

| Field | Value |
| --- | --- |
| **Cites** | specs/17.23 · specs/17.25 · specs/45.16 · arch/10-manager/07 §The values a site supplies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` and `yoke-guardian` built from this tree; an application declaring, in the group of its oneshot `probe`, the parameter `probe.compositor`, a string with an empty default; the oneshot's arguments name it, and it records its arguments and its environment; an instance of it, created with no value given; a session whose environment holds `HYPRLAND_INSTANCE_SIGNATURE` |
| **Action** | from that session, set `probe.compositor` to the variable's value with `yoke instance set-param`, and start the instance |
| **Expected** | the unit was handed the value as its argument; its environment holds no `HYPRLAND_INSTANCE_SIGNATURE` |

## yoke:a-value-the-session-sets.02 — a value set while the instance runs reaches its units when the instance is next started

| Field | Value |
| --- | --- |
| **Cites** | specs/17.24 · specs/17.25 · arch/10-manager/07 §The values a site supplies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the instance of `.01`, running, its oneshot having recorded the first value |
| **Action** | set `probe.compositor` to a second value while it runs; then restart the instance |
| **Expected** | the setting is accepted and the record holds the second value at once; no unit is launched by it; after the restart the oneshot is handed the second value |
