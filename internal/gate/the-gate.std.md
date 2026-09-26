# The gate, and the shared body

| | |
| --- | --- |
| **Feature** | the one validator, as a library: given a document it returns a report — every finding with its code, class, location and message, in phases, naming the phases that did not run — and the shared body both documents are built from, `units`, `channels`, `arbitration` and `policy`, with every field, its type and the rules attached to it |
| **Planning item** | yoke-project/yoke#12 |

## yoke:the-gate.01 — a pass reports everything it found, not the first thing

| Field | Value |
| --- | --- |
| **Cites** | specs/15.30 · specs/14.50 · arch/15-gate/01 §What a pass produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document with five independent problems of shape: an unknown key, a unit with no `kind`, a channel `clients` outside its set, a duration without a unit, and an `env` key in the reserved prefix |
| **Action** | check it |
| **Expected** | one report holding all five refusals — `key.unknown`, `field.required`, `field.value`, `format.duration`, `unit.env.reserved` |

## yoke:the-gate.02 — a finding carries a code, a class, a location and a message naming the value

| Field | Value |
| --- | --- |
| **Cites** | specs/14.50 · arch/15-gate/01 §What a pass produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document at a known path whose unit `acquire` carries `autostart: sometimes` |
| **Action** | check it |
| **Expected** | the finding's code is `field.type`, its class `refusal`, its document the path, its location `units.acquire.autostart`, and its message names `sometimes` |

## yoke:the-gate.03 — a phase whose inputs are untrustworthy does not run, and is named

| Field | Value |
| --- | --- |
| **Cites** | specs/14.50 · arch/15-gate/06 §The phases · arch/15-gate/01 §What a pass produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document that is not YAML; one that parses but has a refusal of shape and a `depends_on` naming no unit |
| **Action** | check each |
| **Expected** | the first reports `document.malformed` and names every later phase as not run; the second reports the shape refusal, no `depends_on.unknown`, and names the internal joins and every phase after them as not run |

## yoke:the-gate.04 — a document with no refusal passes, whatever is weaker in it

| Field | Value |
| --- | --- |
| **Cites** | specs/15.32 · specs/15.31 · arch/15-gate/01 §What it refuses, and what it only reports · arch/15-gate/06 §Phase 5 — weaker arrangements |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a well-formed document whose channel is bound on the loopback address |
| **Action** | check it |
| **Expected** | the report is not refused; it holds one finding, `channel.address.loopback`, of class `weaker`, located at the channel's address |

## yoke:the-gate.05 — a document absent or not YAML is refused at reading

| Field | Value |
| --- | --- |
| **Cites** | specs/14.50 · arch/15-gate/06 §Phase 0 — reading |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a path where nothing is; a file holding `units: [unclosed` |
| **Action** | read and check each |
| **Expected** | `document.unreadable` for the first and `document.malformed` for the second, each naming the document |

## yoke:the-gate.06 — an unknown key is refused at any level

| Field | Value |
| --- | --- |
| **Cites** | specs/14.50 · arch/15-gate/05 §The formats · arch/15-gate/06 §Phase 1 — shape |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document with one unknown key at the top, in a unit, in a unit's `policy`, in a channel, in an address and in an arbitration rule |
| **Action** | check it |
| **Expected** | six `key.unknown` refusals, one at each location, and nothing ignored |

## yoke:the-gate.07 — every field has its type, and a required one its presence

| Field | Value |
| --- | --- |
| **Cites** | specs/14.10 · specs/14.25 · specs/14.29 · specs/14.31 · arch/15-gate/03 §`units` · arch/15-gate/03 §`channels` · arch/15-gate/03 §`address` · arch/15-gate/03 §`arbitration` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | documents missing, in turn, a unit's `kind`, a plugin unit's `plugin`, a channel's `transport` and `clients`, an address's `class`, a routable address's `host`, a loopback address's `port`, and a rule's `prevails` and `over`; and documents with `args` a string, `depends_on` a mapping, `kind: service`, `clients: many` and `class: public` |
| **Action** | check each |
| **Expected** | `field.required` naming each absent field; `field.type` for the two wrongly typed; `field.value` for the three values outside their sets |

## yoke:the-gate.08 — every scalar is written in its form, and typed by the schema and never by the notation

| Field | Value |
| --- | --- |
| **Cites** | specs/14.47 · specs/14.48 · arch/15-gate/05 §The formats · arch/15-gate/03 §`policy` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a policy with `startup_window: 30`, and one with `retention.bytes: 50mb`; a unit whose `manifest_digest` is `md5:ab`; and a unit whose `args` are `[no, 1.0, 01]` |
| **Action** | check each |
| **Expected** | `format.duration`, `format.size` and `format.digest` respectively; the last passes, its arguments read as the strings `no`, `1.0` and `01` |

## yoke:the-gate.09 — a name is validated and never transformed

| Field | Value |
| --- | --- |
| **Cites** | specs/14.49 · specs/12.22 · arch/15-gate/05 §The formats · arch/15-gate/03 §`units` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit named `head/a`, a channel named with a null byte, and a unit named `station.acquire-1` |
| **Action** | check the document |
| **Expected** | `name.not_a_path_component` for the first two; the third passes and reaches the deployment under exactly that name |

## yoke:the-gate.10 — what a unit's kind permits it to carry

| Field | Value |
| --- | --- |
| **Cites** | specs/14.11 · specs/14.14 · specs/14.20 · arch/15-gate/03 §`units` · arch/35-units/04 §What the Core hands the process |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a plugin unit with `needs`; an interface with `migrates_from`; a oneshot whose `env` has `YOKE_TOKEN`; an interface whose `image` is `ghcr.io/yoke/panel:latest` |
| **Action** | check each |
| **Expected** | `unit.needs.on_plugin`, `unit.migrates_from.kind`, `unit.env.reserved` and `unit.image.tagged` |

## yoke:the-gate.11 — how the program is named is the one field that differs between the documents

| Field | Value |
| --- | --- |
| **Cites** | specs/14.12 · specs/14.13 · arch/15-gate/03 §`units` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | in a descriptor: a unit with both `exec` and `image`, one with neither, and one with `exec` and no `digest`; in a composition document: a oneshot with neither, and a plugin unit with neither |
| **Action** | check each |
| **Expected** | `unit.program.ambiguous` twice and `unit.exec.no_digest` in the descriptor; `unit.program.ambiguous` for the oneshot in the composition document, and nothing for the plugin unit, whose executable the Core resolves |

## yoke:the-gate.12 — the needs are a closed set, and a secret is never bound

| Field | Value |
| --- | --- |
| **Cites** | specs/12.36 · specs/14.15 · specs/14.17 · arch/15-gate/03 §`units` · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an interface needing `serial`, one needing `display:main`, one needing `storage` with no name, and one needing `secret:token` with `bind: { token: /etc/token }`; and an interface needing `device:head-a`, `display`, `storage:datasets` and `secret:token`, binding `head-a` |
| **Action** | check each |
| **Expected** | `field.value` for the three needs outside the set's forms and `bind.secret` for the binding; the last passes |

## yoke:the-gate.13 — substitution is permitted in exactly four places

| Field | Value |
| --- | --- |
| **Cites** | specs/14.43 · specs/12.46 · arch/15-gate/05 §Where substitution is permitted |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a reference in an element of `args`, a value of `env`, a value of `bind` and a channel's `address.port`; and one each in `exec`, `image`, `plugin`, `needs` and `depends_on` |
| **Action** | check the document |
| **Expected** | `substitution.place` for each of the five, and none for the four permitted places |

## yoke:the-gate.14 — a reference resolved in neither scope is refused, naming it and its unit

| Field | Value |
| --- | --- |
| **Cites** | specs/14.45 · specs/14.46 · specs/14.18 · arch/15-gate/05 §How a name is resolved |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composition document — which has no parameters — whose interface `panel` binds `head-a`, needs `secret:token`, and has `args` `["${bind.head-a}", "${bind.token}", "${bind.missing}", "${site_label}"]` |
| **Action** | check it |
| **Expected** | two `substitution.unresolved` refusals, for `${bind.missing}` and `${site_label}`, each message naming the reference and `panel`; the bound value and the secret resolve |

## yoke:the-gate.15 — the internal joins, inside one document

| Field | Value |
| --- | --- |
| **Cites** | specs/14.19 · specs/14.24 · specs/14.26 · specs/14.29 · specs/14.33 · specs/14.51 · arch/15-gate/06 §Phase 2 — internal joins |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document where a unit depends on an undeclared unit and another on an interface; a channel names an undeclared unit and another names a oneshot; a channel's transport is `grpc`; a rule names an undeclared channel; and a routable address carries only `security.transport` |
| **Action** | check it |
| **Expected** | `depends_on.unknown`, `depends_on.interface`, `channel.unit.unknown`, `channel.unit.kind`, `channel.transport.unknown`, `arbitration.channel.unknown` and `channel.address.security` |

## yoke:the-gate.16 — policy: written defaults, two scopes, a quantity and never a rule

| Field | Value |
| --- | --- |
| **Cites** | specs/14.21 · specs/14.22 · specs/14.23 · specs/14.35 · specs/14.37 · specs/14.38 · specs/12.16 · arch/15-gate/03 §`policy` · arch/15-gate/03 §The unit's `policy` block |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a document with `policy: { startup_window: 60s, retention: { entries: 0 } }`, a plugin unit `slow` with `policy: { startup_window: 90s }`, a plugin unit `plain`, and a oneshot with `policy: { restart: { on_failure: true } }`; then documents with `restart.on_failure` at the deployment's scope, on a plugin unit, and `restart.attempts` anywhere |
| **Action** | check each, and read the deployment the first produces |
| **Expected** | the first passes: `plain` has a 60 s startup window and every other figure at its written default — `10s`, `3`, `10s`, `5s`, `5m`, `60s`, `7d`, `50MB` — with `entries` held at 0, not the default; `slow` has 90 s and the rest as `plain`; the oneshot restarts on failure. Each of the others is refused with `key.unknown` |

## yoke:the-gate.17 — the heartbeat's tolerance is a whole number of missed intervals, at least one

| Field | Value |
| --- | --- |
| **Cites** | specs/14.35 · arch/15-gate/03 §`policy` · arch/50-plugin-surface/04 §Revocation |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | policies with `heartbeat.tolerance` at `2.5`, at `0`, and at `5` |
| **Action** | check each |
| **Expected** | `field.type` for `2.5` and `field.value` for `0`; the last passes, with a tolerance of 5 |

