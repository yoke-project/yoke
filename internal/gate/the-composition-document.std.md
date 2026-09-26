# The composition document, and the checks a service form needs

| | |
| --- | --- |
| **Feature** | the service form's composing document — the shared body with no head, no parameters and no digests — joined by the gate against the Manifests in the scanned directory and checked against the host it will run on, at the moment that can reach each check |
| **Planning item** | yoke-project/yoke#13 |

## yoke:the-composition-document.01 — a composition document has no head, no parameters and no digests

| Field | Value |
| --- | --- |
| **Cites** | specs/14.5 · specs/14.6 · specs/14.7 · specs/14.8 · specs/14.12 · arch/15-gate/04 §At a glance · arch/15-gate/06 §Phase 1 — shape |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition document carrying `model`, `id`, `version`, `arch`, `core`, `data` and `parameters` at the top; a plugin unit with `exec`, another with `image`; a plugin unit with `manifest_digest`; an interface with `digest`; a oneshot with `migrates_from` |
| **Action** | check it |
| **Expected** | `head.present` for each of the six identity fields, `parameters.present`, `unit.program.on_plugin` twice, `digest.present` twice and `unit.migrates_from.present` |

## yoke:the-composition-document.02 — the Manifests are found in the scanned directory, one per plugin

| Field | Value |
| --- | --- |
| **Cites** | specs/42.1 · specs/42.2 · specs/14.52 · arch/15-gate/04 §The third document · arch/15-gate/06 §Phase 3 — cross-document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a scanned directory holding `com.yoke.station.acquire/manifest.yaml`; a composition document with a plugin unit of `com.yoke.station.acquire` and one of `com.yoke.station.pipeline` |
| **Action** | check it with the directory |
| **Expected** | one refusal, `plugin.manifest.missing`, at the second unit's `plugin`, naming the plugin and the path it was looked for at |

## yoke:the-composition-document.03 — a Manifest the composition names is checked, and its refusals refuse the deployment

| Field | Value |
| --- | --- |
| **Cites** | specs/42.1 · specs/40.18 · specs/40.19 · arch/15-gate/04 §The third document · arch/15-gate/06 §Phase 3 — cross-document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a scanned directory where `com.example.old` is written against model 2 and `com.example.odd` carries an unknown key, and `com.example.unused` is malformed; a composition naming the first two |
| **Action** | check it with the directory |
| **Expected** | `plugin.manifest.model` located in the first Manifest and `key.unknown` in the second, each naming its own document; nothing about the Manifest no unit names |

## yoke:the-composition-document.04 — a plugin unit's needs are joined to its binding in both directions

| Field | Value |
| --- | --- |
| **Cites** | specs/12.35 · specs/40.13 · specs/14.16 · specs/14.17 · specs/14.18 · arch/15-gate/06 §Phase 3 — cross-document · arch/15-gate/03 §`units` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Manifest needing `device:instrument`, `device:spare` and `secret:token`; units of it binding, in turn: `instrument` and `spare` with `args` naming `${bind.instrument}` and `${bind.token}`; only `instrument`; `instrument`, `spare` and `other`; `instrument`, `spare` and `token`; and `instrument` and `spare` with `args` naming `${bind.missing}` |
| **Action** | check each |
| **Expected** | the first passes; then `unit.needs.unbound` naming `spare`, `bind.undeclared` naming `other`, `bind.secret` naming `token`, and `substitution.unresolved` naming `${bind.missing}` |

## yoke:the-composition-document.05 — what the host must hold, checked on the host that will run it

| Field | Value |
| --- | --- |
| **Cites** | specs/12.38 · specs/12.44 · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a host whose executables directory holds `com.example.present`, executable; an interface whose `exec` is absent; a oneshot whose `exec` is there and not executable; a plugin unit of `com.example.absent`, whose Manifest is there and whose executable is not; a plugin unit of `com.example.present` binding `instrument` to a path that does not exist and needing `secret:token`, which the state directory's `secrets/` does not hold |
| **Action** | check it at the start |
| **Expected** | `component.missing` twice, `component.not_executable`, `path.missing` naming the path, and `secret.missing` naming `token` |

## yoke:the-composition-document.06 — the longest socket path the deployment can produce must fit

| Field | Value |
| --- | --- |
| **Cites** | specs/25.36 · arch/15-gate/06 §The two ceilings · arch/30-core/04 §The two ceilings |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a runtime root, and a plugin unit whose Manifest declares one stream, with the unit's and the stream's names chosen so that the longest path — a subscriber's socket, `plugins/<unit>/subscribers/<stream>/` and a subscriber of the bounded width — is 107 characters; then the same with one character more |
| **Action** | check each at the start |
| **Expected** | the first passes; the second gives `socket.path.ceiling`, naming 108 and the ceiling of 107 |

## yoke:the-composition-document.07 — each check runs at the moments that can reach it

| Field | Value |
| --- | --- |
| **Cites** | arch/15-gate/06 §The moments · arch/15-gate/01 §Who invokes it, and when |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composition of case 05, with its scanned directory and host |
| **Action** | check it while composing |
| **Expected** | no host fact is reported, and the report names the host facts as not run; the cross-document phase ran |

## yoke:the-composition-document.08 — the service form's bench passes, with its one weaker arrangement

| Field | Value |
| --- | --- |
| **Cites** | specs/14.1 · specs/14.2 · arch/15-gate/04 §At a glance · arch/97-trace/07 §The composition document |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composition of `arch/97-trace/07` — four copies of `acquire` on four heads, `pipeline` depending on them, `archive`, the panel and two channels under one arbitration rule — with its three Manifests, `acquire` needing `device:instrument`, and a host holding the executables and the four heads |
| **Action** | check it at the start |
| **Expected** | the report is not refused and holds one finding, `channel.address.loopback`; the deployment has the seven units, the two channels and the rule |
