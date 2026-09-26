# The Registry

| | |
| --- | --- |
| **Feature** | the store of authority: one SQLite file in the instance's state directory, opened with four settings and migrated forward at step 6 of the trunk, holding five tables rooted on the plugin — what a Manifest and a registration declared, what an operator authorised, and the history of who decided it — and nothing observed |
| **Planning item** | yoke-project/yoke#11 |

## yoke:the-registry.01 — the Registry is `registry.db` in the instance's state directory

| Field | Value |
| --- | --- |
| **Cites** | specs/30.43 · arch/40-state/01 §Where the files are |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the trunk, in the service form with `state_dir` set, and in the application form under a given `$XDG_STATE_HOME` and instance name |
| **Action** | run the steps up to and including `stores` |
| **Expected** | the Registry is `<state_dir>/registry.db` in the service form and `$XDG_STATE_HOME/yoke/instances/<name>/state/registry.db` in the application form, and the file exists |

## yoke:the-registry.02 — it is opened with the Registry's four settings

| Field | Value |
| --- | --- |
| **Cites** | specs/30.43 · specs/30.44 · arch/40-state/01 §How a store is opened |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Registry opened on a new file |
| **Action** | read the four settings back through the connection the Registry uses |
| **Expected** | journal mode `wal`, synchronous `FULL` (2), foreign keys on, busy timeout 5000 ms |

## yoke:the-registry.03 — a new file is created at the schema this Core implements, stamped in its header

| Field | Value |
| --- | --- |
| **Cites** | specs/30.43 · arch/40-state/01 §The schema is stamped in the file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no file at the path |
| **Action** | open the Registry, close it, and read the file's header number with a separate connection |
| **Expected** | the header's number is the number of steps this Core carries; opening the same file again applies nothing and leaves it there |

## yoke:the-registry.04 — a file at a lower number has the missing steps applied, one transaction each

| Field | Value |
| --- | --- |
| **Cites** | specs/30.43 · arch/40-state/01 §The schema is stamped in the file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file migrated through the first of three steps; then a file whose second step fails |
| **Action** | open the first with all three steps; open the second with the failing step |
| **Expected** | the first ends at 3 with what the second and third steps create present; the second refuses the open, stays at 1, and holds nothing of the failed step — a store is at one number or the next and never between them |

## yoke:the-registry.05 — a file at a higher number is refused, naming both numbers

| Field | Value |
| --- | --- |
| **Cites** | specs/30.43 · specs/30.2 · arch/40-state/01 §The schema is stamped in the file · arch/40-state/01 §The pair |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file whose header number is above what this Core implements; then the trunk, with that file where its Registry goes |
| **Action** | open it; run the trunk |
| **Expected** | the open fails, its error naming the file's number and this Core's, and the file is unchanged; the trunk stops at `stores` and the instance does not start |

## yoke:the-registry.06 — five tables rooted on the plugin, with their columns and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/30.3 · specs/30.5 · specs/30.6 · specs/30.7 · specs/30.8 · arch/40-state/02 §The tables · arch/40-state/02 §The three classes, applied |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a new Registry |
| **Action** | list its tables and each table's columns |
| **Expected** | exactly `plugin` (`id`, `protocol`, `manifest_digest`, `version`, `language`, `sdk`), `policy` (`plugin_id`, `enabled`), `grant` (`plugin_id`, `capability`), `credential` (`plugin_id`, `mode`, `fingerprint`) and `decision` (`seq`, `plugin_id`, `at`, `actor`, `action`, `capability`); no column holds a unit, a state, a session, a count, a first- or last-seen time, or an executable's digest |

## yoke:the-registry.07 — a row with no plugin fails where it is written

| Field | Value |
| --- | --- |
| **Cites** | specs/30.3 · arch/40-state/01 §How a store is opened · arch/40-state/02 §The root, and what a key means |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a new Registry |
| **Action** | grant a capability, and enable, for a plugin never declared |
| **Expected** | both fail, and nothing is written — no policy, no grant, no decision |

## yoke:the-registry.08 — declaring a plugin writes what the Manifest says, and grants nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/30.8 · specs/30.14 · specs/30.15 · specs/30.18 · arch/40-state/02 §`plugin` — what the artifact says it is · arch/40-state/02 §`credential` — how a plugin proves it is itself |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a new Registry |
| **Action** | declare a plugin with an identity, a protocol and a Manifest digest, and read it back |
| **Expected** | the plugin carries the three; version, language and SDK are empty; it is enabled with no grant; its credential's mode is `bootstrap` with no fingerprint; the history is empty — no decision was taken |

## yoke:the-registry.09 — declaring it again updates what was declared and touches nothing decided

| Field | Value |
| --- | --- |
| **Cites** | specs/30.6 · specs/30.18 · arch/30-core/07 §Reconciling, in the form that can |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared plugin an operator disabled and granted one capability |
| **Action** | declare it again with a new protocol and a new Manifest digest |
| **Expected** | the plugin carries the new protocol and digest; it is still disabled, still holds its grant, and its history is the two decisions and no more |

## yoke:the-registry.10 — a registration records version, language and SDK, and nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/30.8 · arch/40-state/02 §`plugin` — what the artifact says it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared plugin |
| **Action** | record a registration's version, language and SDK line |
| **Expected** | the plugin carries the three, its protocol and Manifest digest unchanged; policy, grants and history are untouched |

## yoke:the-registry.11 — each change of authority writes one decision, naming who and when

| Field | Value |
| --- | --- |
| **Cites** | specs/30.4 · specs/32.27 · arch/40-state/02 §`decision` — the history beside the policy · arch/40-state/02 §`grant` — the capabilities in force |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared plugin, and a clock the test sets |
| **Action** | disable it, grant a capability, withdraw it, enable it — each as a named actor |
| **Expected** | four decisions in order — `disabled`, `granted` with the capability, `withdrawn` with the capability, `enabled` — each with the actor and the clock's instant; the grant is a row while it holds and absent once withdrawn |

## yoke:the-registry.12 — an operation whose effect is already true changes nothing and records nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/30.4 · arch/40-state/02 §`decision` — the history beside the policy |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared plugin, enabled, holding one grant |
| **Action** | enable it; grant the capability it holds; withdraw one it does not hold |
| **Expected** | each succeeds and says nothing changed; the history is as it was |

## yoke:the-registry.13 — nothing removes a plugin, and disabling and withdrawing leave the record

| Field | Value |
| --- | --- |
| **Cites** | specs/30.19 · specs/30.20 · specs/30.21 · arch/40-state/02 §Nothing is removed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a declared plugin, granted a capability, which nothing declares any more |
| **Action** | withdraw the grant and disable it; then close and reopen the Registry |
| **Expected** | the plugin, its policy, its credential and its three decisions are all there, disabled and with no grant; the Registry offers no operation that removes a plugin |

## yoke:the-registry.14 — a Core that cannot open its Registry does not start

| Field | Value |
| --- | --- |
| **Cites** | specs/30.2 · arch/40-state/01 §The pair · arch/30-core/03 §The fatal boundary is a rule and not a number |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, its `state_dir` a regular file where a directory should be; then a writable `state_dir` |
| **Action** | start it each time |
| **Expected** | the first exits non-zero before `ready`, naming the step `stores`; the second is ready and `registry.db` exists under `state_dir` |
