# Advice and waivers

| | |
| --- | --- |
| **Feature** | the composer's second class of finding: the five advisories as the set stands — `retention.budget.small`, `oneshot.restart.unchecked`, `arbitration.panel.starved`, `parameter.label.overclaims`, `data.range.unstated` — each raised on its stated condition at one location, class `advice`, only where the gate refused nothing, printed by `check` after the gate's findings and never changing its exit status; `waive <code> --at <location> --reason <text>` writing a waiver to `advisories.yaml`, which no package holds, after refusing an unknown code, an empty reason, a place not advised now and a duplicate; a waived item left out of the report and counted; `waivers` listing every waiver with its reason, and marking one that matches nothing `stale` without dropping it |
| **Planning item** | yoke-project/yoke#283 |

## yoke:advice-and-waivers.01 — each advisory is raised on its condition, at its location, and advice changes nothing about the exit status

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/04 §The set as it stands · arch/20-composer/04 §The two classes must not swap places |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project the gate refuses nothing in, holding: a `plugin` unit whose retention keeps `500` entries; a `oneshot` stating `restart.on_failure: true` and needing `storage:datasets`, under a head stating no `data`; a channel `remote` on loopback, taking its port from a parameter labelled *Diagnostics port, loopback only*, prevailing over the local-socket channel `panel` |
| **Action** | `check` it; then the same project with each of the five combinations removed in turn; then the first with a gate refusal added |
| **Expected** | the first prints the five codes, class `advice`, at `units.<plugin>.policy.retention.entries`, `units.<oneshot>.policy.restart.on_failure`, `arbitration.0`, `parameters.remote.listen_port.label` and `units.<oneshot>`, after the gate's findings, and exits `0`; each variant lacks exactly the code removed; with a refusal, no advice is printed and it exits `1` |

## yoke:advice-and-waivers.02 — the thresholds and the words are exact

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/04 §The set as it stands |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `plugin` units whose retention is `age: 1d`, `age: 23h`, `bytes: 5MB`, `bytes: 4MB`, `entries: 10000`, `entries: 0`, and the same `age: 23h` on a `oneshot`; channel port parameters labelled *SECURE port*, *insecure port* and *only*, on a loopback channel, and *only* on a routable channel, whose caller is authenticated |
| **Action** | check each |
| **Expected** | `retention.budget.small` for `23h` and `4MB` on a `plugin` unit and for nothing else; `parameter.label.overclaims` for *SECURE port* and *only* on loopback, and for nothing else |

## yoke:advice-and-waivers.03 — a waiver silences one place, with its reason, and a stale one is listed and never dropped

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/04 §Waiving an item · arch/20-composer/02 §The two sidecars |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01` |
| **Action** | `waive` an unknown code; a known code with an empty reason; a known code at a place it is not raised; `parameter.label.overclaims` at its place with a reason; the same again; then `check`; `waivers`; rewrite the label so it is no longer raised and run `waivers` and `check` again; build the project |
| **Expected** | the first three and the fifth are refused and `advisories.yaml` is unchanged by them; the fourth writes the code, the place, the reason and when; `check` prints the four other codes and says one item is waived, its noun agreeing with its number — `1 advisory item`; `waivers` prints the waiver with its reason; after the rewrite it marks it `stale` and keeps it, and `check` waives nothing; the package holds no `advisories.yaml` |
