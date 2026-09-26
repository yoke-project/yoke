# Discovery in the service form

| | |
| --- | --- |
| **Feature** | the one way a declaration enters a service-form deployment: a periodic scan of the Plugin directory that makes a Plugin exist — declared in the Registry, authorisable, not running — and the composition document in force, joined through the gate at step 8, which says what runs; with what survives a failure in each |
| **Planning item** | yoke-project/yoke#10 |

## yoke:discovery.01 — a scan declares every Manifest it finds, as it reads it

| Field | Value |
| --- | --- |
| **Cites** | specs/20.42 · specs/20.43 · specs/20.44 · specs/30.8 · arch/30-core/07 §The one way in · arch/30-core/07 §What it reads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin directory holding the Manifests of two plugins, one subdirectory each; a new Registry |
| **Action** | scan it |
| **Expected** | both plugins are in the Registry with their identity, their protocol and the SHA-256 of their Manifest's bytes as read; both are available |

## yoke:discovery.02 — a Manifest that fails its check is logged and skipped, and the others are still declared

| Field | Value |
| --- | --- |
| **Cites** | specs/20.42 · specs/20.50 · arch/30-core/07 §What it reads · arch/50-plugin-surface/02 §What is refused, before anything runs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin directory holding one valid Manifest, one that does not parse, and one carrying `endpoint` |
| **Action** | scan it |
| **Expected** | the valid plugin is declared and available; the other two are neither, and the process logger holds a line for each naming its path and what was refused |

## yoke:discovery.03 — a later scan reconciles in both directions, and removes nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/20.48 · specs/50.11 · specs/30.19 · specs/30.21 · arch/30-core/07 §Reconciling, in the form that can · arch/40-state/02 §Nothing is removed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a scanned directory with plugins `a` and `b`; then `a`'s Manifest changed to declare another stream, `b`'s removed, and `c`'s added |
| **Action** | scan again |
| **Expected** | `c` is declared and available; `a` carries its new Manifest digest and the Manifest discovery holds for it is the new one; `b` is still in the Registry, with its record untouched, and is no longer available |

## yoke:discovery.04 — a Plugin installed and not composed is present and idle

| Field | Value |
| --- | --- |
| **Cites** | specs/20.46 · arch/30-core/07 §Availability and composition are two questions |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a service-form trunk whose Plugin directory holds `com.example.composed` and `com.example.idle`, and whose composition names a unit of the first only |
| **Action** | run the trunk to readiness |
| **Expected** | both plugins are declared in the Registry; the units handed to the supervisor are the composed one's alone |

## yoke:discovery.05 — the composition says what runs, each unit with its own identity and resolved arguments

| Field | Value |
| --- | --- |
| **Cites** | specs/20.47 · specs/20.45 · specs/14.18 · specs/14.44 · arch/30-core/07 §One identifier becomes two · arch/35-units/04 §The plan |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a service-form trunk whose composition runs two copies of one plugin, each binding `instrument` to a different path and passing `${bind.instrument}` as an argument, and a oneshot with `autostart: false` |
| **Action** | run the trunk to readiness |
| **Expected** | the two copies are handed to the supervisor as two units under their own names, of the one plugin, each executable resolved from the plugin's identity in the executables directory, each argument its own bound path as one element; the oneshot is not handed over |

## yoke:discovery.06 — a composition the gate refuses is reported, and the Core is still ready

| Field | Value |
| --- | --- |
| **Cites** | specs/20.45 · specs/20.50 · arch/30-core/03 §The fatal boundary is a rule and not a number · arch/30-core/07 §What it reads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a service-form trunk whose Plugin directory holds one valid Manifest, and whose composition names a plugin with no Manifest |
| **Action** | run the trunk |
| **Expected** | it reaches readiness; the process logger holds the finding `plugin.manifest.missing` with its location; the valid plugin is declared; no unit is handed to the supervisor |

## yoke:discovery.07 — the scan runs again at the configured interval

| Field | Value |
| --- | --- |
| **Cites** | specs/20.44 · arch/30-core/07 §What it reads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running scan at an interval of 100 ms over an empty directory |
| **Action** | write a Manifest into the directory |
| **Expected** | within a second the plugin is declared and available, with nobody having asked |

## yoke:discovery.08 — the Core declares what it finds and runs what the composition says

| Field | Value |
| --- | --- |
| **Cites** | specs/20.42 · specs/20.44 · specs/20.45 · arch/30-core/07 §What it reads · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, its `core.yaml` naming a Plugin directory with one valid Manifest and one that does not parse; the composition in force running a oneshot that prints a line |
| **Action** | start it, and stop it once it is ready and the line is there |
| **Expected** | the Core is ready; its output holds a line naming the Manifest that did not parse, and the oneshot's line attributed to its unit; `registry.db` holds the valid plugin and not the other |
