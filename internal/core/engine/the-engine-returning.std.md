# What runs under the label, and the engine's return

| | |
| --- | --- |
| **Feature** | what the supervisor asks of an engine that went quiet: what runs under the instance's label, each container with its unit, its incarnation and whether it is running or the status it ended with; and a notice each time the engine's socket appears at its path, which is how its return is learnt without looking for it |
| **Planning item** | yoke-project/yoke#171 |

## yoke:the-engine-returning.01 — what runs under the instance's label is asked of the engine, an ended container with its status

| Field | Value |
| --- | --- |
| **Cites** | specs/45.24 · specs/45.26 · arch/30-core/05 §When the source of facts goes quiet · arch/35-units/05 §Driving the engine |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine holding two containers under the label of the instance `bench`: one running, of the unit `panel` in its second incarnation, and one ended with status 3, of the unit `calibrate` in its first |
| **Action** | list what runs under the label of `bench` |
| **Expected** | the engine is asked for every container, ended or not, carrying the label `dev.yoke-project.instance=bench` alone; the answer holds both, each with its identity, its unit and its incarnation, the first running and the second ended with status 3 |

## yoke:the-engine-returning.02 — the engine's socket appearing at its path is a notice, each time

| Field | Value |
| --- | --- |
| **Cites** | specs/20.18 · arch/30-core/05 §Facts arrive; it does not go looking · arch/30-core/05 §When the source of facts goes quiet |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an engine reached on a socket of the test's, which then goes away |
| **Action** | wait for the engine's return; create another file beside the socket; bind the socket again; remove it and bind it a third time; then end the wait |
| **Expected** | no notice for the other file; one notice for each time the socket is bound; once the wait ends, what carried the notices is closed |
