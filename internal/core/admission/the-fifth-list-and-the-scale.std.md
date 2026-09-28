# Occurrences in the declared surface, and a grade off the scale

| | |
| --- | --- |
| **Feature** | the registration's fifth list — the occurrences a process claims — compared at stage 6 like the other four and carried in the granted scope and the withheld items; an occurrence a unit reports published as the Core's event on the unit's life, at the grade the unit declared; and a declared severity or health grade above 99 read as 99, with a warning naming the value declared and no error to the unit |
| **Planning item** | yoke-project/yoke#107 |

## yoke:the-fifth-list-and-the-scale.01 — stage 6 compares the occurrences a process claims with its Manifest's

| Field | Value |
| --- | --- |
| **Cites** | specs/50.16 · arch/50-plugin-surface/03 §What the request claims · arch/50-plugin-surface/03 §The nine stages |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin whose Manifest declares the occurrences `calibration.drift` and `head.fault` |
| **Action** | a unit registers claiming only `calibration.drift`; another claiming both |
| **Expected** | the first is refused at declaration consistency with `admission.consistency.divergent`, its message naming the occurrences; the second is accepted |

## yoke:the-fifth-list-and-the-scale.02 — the granted scope and the withheld items carry occurrences

| Field | Value |
| --- | --- |
| **Cites** | specs/50.30 · specs/50.26 · arch/50-plugin-surface/03 §The grant is an intersection, computed once · arch/50-plugin-surface/08 §The registration exchange |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same plugin, with only the capability governing `calibration.drift` authorised |
| **Action** | a unit registers claiming both occurrences |
| **Expected** | it is accepted with restrictions; the granted scope names `calibration.drift` among its occurrences, and the withheld items name `head.fault` and the capability governing it |

## yoke:the-fifth-list-and-the-scale.03 — an occurrence a unit reports is published on the unit's life, at the grade declared

| Field | Value |
| --- | --- |
| **Cites** | specs/31.49 · specs/31.15 · arch/45-events/06 §The set · arch/50-plugin-surface/05 §The event family is where a unit declares a severity |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session of a unit's third life, granted `calibration.drift` |
| **Action** | the unit reports `calibration.drift` at 70, with a line and a detail |
| **Expected** | the Core publishes one `unit.occurrence.reported` about the unit's third life, with the class in its occurrence field, severity 70, the line and the detail as sent, and the unit as its actor |

## yoke:the-fifth-list-and-the-scale.04 — a severity above 99 is read as 99, with a warning, and the report is accepted

| Field | Value |
| --- | --- |
| **Cites** | specs/31.53 · arch/45-events/02 §Nobody restates a declared value · arch/50-plugin-surface/08 §The eight payloads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same Session |
| **Action** | the unit reports `calibration.drift` at 150 |
| **Expected** | the Core publishes it at 99; its log records a warning naming the unit and the value 150; the unit receives nothing |

## yoke:the-fifth-list-and-the-scale.05 — a health grade above 99 is read as 99, with a warning, and still counts as a heartbeat

| Field | Value |
| --- | --- |
| **Cites** | specs/31.53 · specs/50.65 · arch/50-plugin-surface/08 §The eight payloads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session whose terms are a heartbeat every 100 ms, three misses tolerated |
| **Action** | the unit sends health reports graded 200, every 100 ms, for a second |
| **Expected** | the Core's log records a warning naming the unit and the value 200; the unit receives nothing and its Session is not revoked |

## yoke:the-fifth-list-and-the-scale.06 — through the Core, a unit that leaves out its occurrences is refused at stage 6

| Field | Value |
| --- | --- |
| **Cites** | specs/50.16 · arch/50-plugin-surface/03 §The nine stages |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a plugin whose Manifest declares `calibration.drift`, and a composition running one unit of a program that registers claiming no occurrence |
| **Action** | start the Core |
| **Expected** | the unit is refused at declaration consistency with `admission.consistency.divergent` |
