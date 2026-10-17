# The authoring verbs

| | |
| --- | --- |
| **Feature** | the verbs that write a project's description: `add unit`, `add channel`, `add parameter`, `set` by the path a finding carries, `arbitrate`; each refusing, and leaving the document as it was, where the gate refuses what it would write; and the comparison, made as a test, of what the verbs can write against every field the gate's schema names for either document |
| **Planning item** | yoke-project/yoke#281 |

## yoke:the-authoring-verbs.01 — `add unit` writes a unit with its program, its arguments and its dependencies, the digest computed

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/20-composer/02 §The working directory |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project made by `init`, with a program at `components/setup` |
| **Action** | `add unit store --kind oneshot --exec components/setup --arg one --arg two`; then `add unit acquire --kind oneshot --exec components/setup --depends-on store`; then `add unit store` again; then `add unit late --kind oneshot --exec components/missing` |
| **Expected** | the first two exit `0` and write each unit with its kind, its program, the program's `sha256:` digest, its arguments and its dependency, and the project passes `check`; the third and fourth exit `1`, naming the unit and what refused it, and the descriptor is byte for byte what it was before each |

## yoke:the-authoring-verbs.02 — `add channel` writes a channel, its transport, its cardinality and its address

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/15-gate/03 §`address` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project holding one unit |
| **Action** | `add channel panel --transport http+ws --clients single`; then `add channel remote --transport local --clients multiple --class routable --host 0.0.0.0 --port 8443`; then `add channel bad --transport carrier-pigeon --clients single` |
| **Expected** | the first two exit `0`; the second's address states both security fields, a routable address being declarable with them alone; the third exits `1` naming the transport, and the descriptor is unchanged |

## yoke:the-authoring-verbs.03 — `add parameter` writes a declaration with its type, its domain, whether it is required and its label

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project made by `init` |
| **Action** | `add parameter shared.label --type string --required --label "Site label"`; then `add parameter shared.rate --type integer --range "[1, 10]" --default 5 --label Rate`; then `add parameter shared.label --type string --label Again` |
| **Expected** | the first two exit `0` and write each declaration as given; the third exits `1`, the parameter being declared, and nothing changes |

## yoke:the-authoring-verbs.04 — `set` writes a field by the path a finding carries, and nothing the gate refuses

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/20-composer/01 §What it must be able to write |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project holding a unit `store`, whose descriptor carries a comment |
| **Action** | `set units.store.policy.startup_window 45s`; then `set units.store.args "[a, b]"`; then `set units.store.policy.startup_window soon`; then `set units.store.colour red` |
| **Expected** | the first two exit `0`, writing the duration and the list at their paths and keeping the comment; the third exits `1` with the finding the gate gives at that path; the fourth exits `1` naming a path the description model does not have; neither of the last two changes the descriptor |

## yoke:the-authoring-verbs.05 — `arbitrate` writes one precedence relation

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §The verbs · arch/15-gate/03 §`arbitration` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project holding a unit and three channels, `panel`, `remote` and `bench` |
| **Action** | `arbitrate panel over remote bench`; then `arbitrate panel over nowhere` |
| **Expected** | the first exits `0` and appends one rule, `panel` prevailing over the other two; the second exits `1` naming the channel that is not declared, and nothing changes |

## yoke:the-authoring-verbs.06 — every field either document can say is written by a verb

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/01 §What it must be able to write · specs/95.9 · specs/95.13 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | every field the gate's schema names for a descriptor, and for a composition document; a project, and a composition document, each holding the unit, channel, group and rule a field belongs to |
| **Action** | write each field with a verb, into the descriptor and — for a field a composition document says — into the composition document named by `--file` |
| **Expected** | each verb exits `0` and the field is at its path afterwards; a field of the schema no verb writes fails the case by name |

## yoke:the-authoring-verbs.07 — `add unit` writes the bindings its needs take, and is judged with them

| Field | Value |
| --- | --- |
| **Cites** | specs/12.35 · arch/20-composer/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project made by `init`, with a program at `components/setup` and the Manifest of a Plugin declaring two device needs, `instrument` and `lamp` |
| **Action** | `add unit head --kind plugin --plugin com.example.head --exec components/setup` with neither binding; then with `--bind instrument=/dev/spectro-a --bind lamp=/dev/lamp-a` |
| **Expected** | the first exits `1` naming `unit.needs.unbound`, and the descriptor is byte for byte what it was; the second exits `0` and writes the unit with both bindings under `bind`, and the project passes `check` |

