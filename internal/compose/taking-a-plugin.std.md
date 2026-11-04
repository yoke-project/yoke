# Taking a Plugin into an application

| | |
| --- | --- |
| **Feature** | `yoke-compose add plugin <path> [--as <unit>] [--yes]` in an application project: the Plugin's file and Manifest checked, the descriptor's architecture checked against those it lists, the source, the commit and the command shown and asked before the author's build runs, the build run for the descriptor's architecture, the executable placed at `components/plugins/<unit>` and the Manifest at `manifests/<id>.yaml`, the unit declared with its executable's digest pinned, the project left unchanged on any refusal, and the Plugin recorded in `plugins.yaml` beside the descriptor — no tag, the commit where the path is a git tree, whether it was clean — which nothing packages; `remove units.<unit>` takes its line out; nothing written grants anything |
| **Planning item** | yoke-project/yoke#401 |

## yoke:taking-a-plugin.01 — a Plugin taken from a path is built, placed, declared, pinned and recorded

| Field | Value |
| --- | --- |
| **Cites** | specs/95.35 · specs/95.41 · arch/20-composer/07 §Taking a Plugin into an application · arch/20-composer/07 §`plugins.yaml` · arch/20-composer/02 §The two sidecars |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an application project made by `init`, its descriptor's `arch` `x86_64`; a Plugin project of `id` `com.example.greeter` and `program` `greeter` in a git tree with no change uncommitted, listing `x86_64`, whose command writes a fixed executable; the composer's clock fixed |
| **Action** | `add plugin <path> --yes` in the application project; then `build` it into a package |
| **Expected** | exit 0; `components/plugins/greeter` is the executable the build wrote and `manifests/com.example.greeter.yaml` is byte for byte the Manifest kept in the source; the descriptor declares a unit `greeter` of that Plugin with the executable's SHA-256 pinned; `check` refuses nothing; `plugins.yaml` holds one entry — the `id`, the unit, the source as given, `tag: null`, the tree's commit, `clean: true`, `arch: x86_64`, the digest and the fixed time; the package holds the executable and the Manifest and no `plugins.yaml` |

## yoke:taking-a-plugin.02 — `--as` names the unit, and a tree with uncommitted changes is recorded as not clean

| Field | Value |
| --- | --- |
| **Cites** | specs/95.35 · specs/95.41 · arch/20-composer/07 §The verbs · arch/20-composer/07 §`plugins.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the projects of `.01`, the Plugin's source given a change not committed; and a second Plugin project, outside any git tree |
| **Action** | `add plugin <path> --as hello --yes`; then `add plugin` the second with `--yes` |
| **Expected** | the first unit is `hello`, placed at `components/plugins/hello`, its entry carrying `clean: false`; the second's entry carries `tag: null` and no commit; both are declared and pinned as in `.01` |

## yoke:taking-a-plugin.03 — the composer shows what it will run and asks before the author's build

| Field | Value |
| --- | --- |
| **Cites** | specs/95.43 · arch/20-composer/07 §Taking a Plugin into an application |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the projects of `.01`; the Plugin's command writing a mark file when it runs |
| **Action** | `add plugin <path>` with no `--yes`, answering no on standard input; again, answering yes; again, with no `--yes` and standard input not a terminal and empty |
| **Expected** | each time the source, the commit and the command verbatim are printed before any build; after no, exit non-zero naming the refusal, no mark file, the project byte for byte as it was; after yes, as `.01`; with no answer, refused as declined, no mark file, the project unchanged |

## yoke:taking-a-plugin.04 — what taking refuses leaves the project as it was

| Field | Value |
| --- | --- |
| **Cites** | specs/95.35 · arch/20-composer/07 §Taking a Plugin into an application |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the application project of `.01`; a path holding no `yoke-plugin.yaml`; a Plugin project the validator refuses; one listing only `aarch64`; one whose command exits non-zero; a file not accounted for already at `components/plugins/greeter`; a unit `greeter` already declared; the Plugin taken once as `greeter` |
| **Action** | `add plugin --yes` each, the last taking the same Plugin again as `greeter` |
| **Expected** | each exits non-zero, naming respectively the missing file, the validator's finding, the descriptor's `x86_64` and the architectures listed — before any build runs —, the command's exit status, the path of the file found, the unit, and the Plugin already taken (with `update plugin` as the way to another revision); the descriptor, `components/`, `manifests/` and `plugins.yaml` are byte for byte what they were |

## yoke:taking-a-plugin.05 — removing the unit takes the Plugin's line out of `plugins.yaml`

| Field | Value |
| --- | --- |
| **Cites** | specs/95.35 · arch/20-composer/07 §The verbs · arch/20-composer/07 §`plugins.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the application project of `.01` after two Plugins were taken, as `greeter` and `hello` |
| **Action** | `remove units.hello`; then `remove units.greeter` |
| **Expected** | after the first, `plugins.yaml` holds `greeter`'s entry alone and the unit `hello` is gone as `remove` takes any unit; after the second, the project has no `plugins.yaml` |

## yoke:taking-a-plugin.06 — taking a Plugin grants it nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/95.36 · arch/20-composer/07 §Taking a Plugin into an application |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the application project of `.01` before and after the Plugin is taken, its Manifest declaring a capability |
| **Action** | compare the two projects; build the package of each |
| **Expected** | what changed is the unit's declaration, its pinned digest, the two placed files and `plugins.yaml`; nothing written names a grant, an authorisation or the capability as granted, and the package's grant-bearing files are what they were |

## yoke:taking-a-plugin.07 — a Plugin so taken is admitted by a Core

| Field | Value |
| --- | --- |
| **Cites** | specs/95.35 · arch/20-composer/07 §Taking a Plugin into an application |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the reference tier's host; an application project made by `init`; a Plugin project in Go built with the family's library, whose Manifest is kept in its source |
| **Action** | `add plugin <path> --yes`; `build` the package; install it and bring its instance up |
| **Expected** | the unit's Plugin is admitted — its Manifest matches its declaration — and reaches running; `plugins.yaml` is not in what was installed |
