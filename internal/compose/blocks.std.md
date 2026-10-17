# Blocks

| | |
| --- | --- |
| **Feature** | the composer's blocks: `block add <source> --as <name>` expands a fragment — one unit, its group's parameters, the channels it offers — into ordinary declarations of the project's descriptor, the unit under the placed name, its parameter group renamed with it and the channels named after it following, and records the placement as one line of `blocks.yaml` — the block, the name, the source, the digest of the block as read, and when — which nothing packages; a name already used, a block of another model, parameters declared into `shared` and whatever the gate refuses once the expansion is written are refused, leaving the project as it was; a source is a path the invocation names, and nothing else is searched; `block save <unit> <destination>` writes the reverse, and leaves the project untouched |
| **Planning item** | yoke-project/yoke#282 |

## yoke:blocks.01 — placing a block expands it into ordinary declarations, and records where it came from

| Field | Value |
| --- | --- |
| **Cites** | specs/95.11 · specs/95.12 · specs/95.29 · arch/20-composer/03 §Placing one · arch/20-composer/03 §Reuse is by copy · arch/15-gate/05 §How a name is resolved |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project made by `init`, holding a program at `components/acquire`; a block `acquisition-head` of the project's model, whose unit runs that program with `${instrument}` in its `bind`, whose group declares `instrument`, and which offers a channel `acquisition-head` served by its unit; the composer's clock fixed |
| **Action** | `block add` the block's file `--as acquire-2`; then place the block again from the directory holding it, `--as acquire-3` |
| **Expected** | the descriptor declares the unit `acquire-2` with the program's digest beside it and `${instrument}` unchanged, the group `parameters.acquire-2` holding `instrument`, and the channel `acquire-2` served by `acquire-2`; nothing in it names the block; `check` refuses nothing; `blocks.yaml` holds two lines, the first naming `acquisition-head`, `acquire-2`, the source as given, the SHA-256 of the block's file and the fixed time; a package built from the project holds no `blocks.yaml` |

## yoke:blocks.02 — what placement refuses leaves the project as it was

| Field | Value |
| --- | --- |
| **Cites** | specs/95.29 · arch/20-composer/03 §Placing one · arch/20-composer/03 §What a block holds · arch/20-composer/03 §Where a block comes from |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project and the block of `.01`, the block placed once as `acquire-2`; a block of model `0`; a block declaring parameters into `shared`; a block whose unit's kind the gate refuses; a directory holding two blocks; a path that does not exist |
| **Action** | place the block `--as acquire-2`; place it `--as` a name a channel already holds; place each of the other four blocks; place the path that does not exist |
| **Expected** | each is refused, naming respectively the unit, the channel, the model, `shared`, the gate's finding, the directory's two files, and the path; the descriptor and `blocks.yaml` are byte for byte what they were |

## yoke:blocks.03 — saving a unit writes it as a block, and placing that block again gives the same declarations under the new name

| Field | Value |
| --- | --- |
| **Cites** | specs/95.10 · arch/20-composer/03 §Reuse is by copy · arch/20-composer/03 §What a block holds |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01` after the block was placed as `acquire-2` |
| **Action** | `block save acquire-2` to a new file; save it again to that same file, and to a file in a directory that does not exist; then `block add` the saved file `--as acquire-4` |
| **Expected** | the saved file names the block `acquire-2`, states the project's model, and holds the unit, the group's `instrument` and the channel served by `acquire-2`; the project is byte for byte what it was; the second save is refused, naming the file, and the third is refused, exit 1, naming the directory that is not there; the placement declares `acquire-4`, its group and its channel exactly as `acquire-2`'s are declared, under the new name |
