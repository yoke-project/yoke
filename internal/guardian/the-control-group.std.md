# The control group

| | |
| --- | --- |
| **Feature** | the instance's control group: the systemd slice `yoke-<instance>.slice`, in which the guardian executes the Core, so that the Core, its host units and the containers the Core places there all sit in it; emptied in one operation, with nothing enumerated, after a stop, after a silence, and before an instance is brought back; and, where the account's systemd delegates nothing, the declared weaker fallback — a process group signalled and containers reaped by label |
| **Planning item** | yoke-project/yoke#233 |

## yoke:the-control-group.01 — the Core runs in the instance's slice, and what escaped it is ended when the Core dies

| Field | Value |
| --- | --- |
| **Cites** | specs/17.44 · specs/17.45 · arch/25-guardian/05 §Who creates it, and what goes in · arch/25-guardian/05 §Emptying it |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built from this tree; the account's own systemd; an instance of a name no other run uses, whose one unit starts a process in a session of its own that outlives its parent and ignores `SIGTERM` |
| **Action** | start the guardian; once the instance is observable, end its Core with `SIGKILL` |
| **Expected** | the Core and the escaped process are in `yoke-<instance>.slice`, and no report of the gate names `confinement.no_cgroup_delegation`; once the Core is dead, the escaped process ends too, before the instance is brought back |

## yoke:the-control-group.02 — a container the Core launched is in the slice, and ends with it

| Field | Value |
| --- | --- |
| **Cites** | specs/17.44 · specs/17.45 · arch/25-guardian/05 §What it is for · arch/35-units/05 §Driving the engine |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | the arrangement of `.01`, with the engine named by the configuration; the instance's one unit a container that runs until it is ended |
| **Action** | start the guardian; once the container runs, end the Core with `SIGKILL` |
| **Expected** | the container's processes are in the instance's slice; once the Core is dead the container is no longer running. Not applicable under Docker, whose root-owned daemon places a container under the system's manager and not the account's |

## yoke:the-control-group.03 — without delegation, the fallback is declared weaker and signals the Core's process group

| Field | Value |
| --- | --- |
| **Cites** | specs/17.46 · arch/25-guardian/05 §Where a host cannot provide one · arch/15-gate/06 §Phase 5 — weaker arrangements |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an account whose systemd cannot be asked to run anything — no `systemd-run` on the `PATH`; a Core that starts a child of its own and says it is alive |
| **Action** | start the guardian; once the instance is observable, write it stopped and tell the guardian |
| **Expected** | the guardian's output names `confinement.no_cgroup_delegation` as weaker before the Core is executed; the Core leads a process group of its own; once stopped, its child has ended as well |
