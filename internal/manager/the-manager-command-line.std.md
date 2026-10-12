# The manager's command line

| | |
| --- | --- |
| **Feature** | `yoke`, one command whose lifecycle verbs are namespaced and whose client verbs are grouped by subject; an exit status that tells a refusal from a fault; and the client verbs — one per operation of the administrative contract — reaching a running instance through the published administrative library, on the operator projection, the instance a selector and never a namespace |
| **Planning item** | yoke-project/yoke#224 |

## yoke:the-manager-command-line.01 — an exit status tells a refusal from a fault

| Field | Value |
| --- | --- |
| **Cites** | specs/15.6 · specs/15.31 · arch/10-manager/01 §What it reports |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a verb that succeeds, a client verb against an instance that is not running, and a word that is no verb |
| **Action** | run each |
| **Expected** | the first exits 0; the second exits 1, reporting that the instance is not running; the third exits 2, naming the word and the verbs there are |

## yoke:the-manager-command-line.02 — the instance is a selector, and an instance's name chooses nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/15.69 · arch/10-manager/01 §The client verbs · arch/60-administrative-surface/03 §Reaching the socket is the whole of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `XDG_RUNTIME_DIR` set; the names `bench`, `../bench` and an empty one given to `--instance`; then no `--instance` with `YOKE_CONFIG` naming a `core.yaml` |
| **Action** | resolve the operator projection for each |
| **Expected** | `bench` is `$XDG_RUNTIME_DIR/yoke/bench/operator.sock`; the other two names are refused naming them; with no selector the address is under the `runtime_dir` that `core.yaml` states |

## yoke:the-manager-command-line.03 — a client verb reaches a running instance and reports what the operation answered

| Field | Value |
| --- | --- |
| **Cites** | specs/15.5 · specs/15.10 · specs/15.12 · arch/10-manager/01 §The client verbs · arch/10-manager/01 §Which projection a verb speaks to · arch/60-administrative-surface/04 §What every answer carries |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core running a composition of one Plugin unit, reached through the `core.yaml` it was started with |
| **Action** | run `yoke core status`, `yoke unit list`, `yoke plugin show`, `yoke plugin disable`, `yoke plugin disable` again, `yoke plugin enable`, `yoke unit restart`, and `yoke log` |
| **Expected** | each exits 0; the status names the instance ready; the listing names the unit and its state; the plugin is shown with its enablement; the first disable answers what it replaced, `immediately`, and the unit it ended; the second says nothing changed; the restart names the incarnation ended and the one begun; the log prints entries of that unit |

## yoke:the-manager-command-line.04 — `yoke app` installs a package, and lists and shows what is installed

| Field | Value |
| --- | --- |
| **Cites** | specs/15.46 · specs/15.50 · arch/10-manager/01 §`yoke app` — artifacts · arch/10-manager/05 §The order · arch/10-manager/04 §What is read from the tree |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a configuration naming an install root and a Core built from this tree, no trust store on the host, and an unsigned package built by the composer |
| **Action** | run `yoke app install` with the package, then again; then `yoke app list` and `yoke app show com.yoke.station`; then `yoke app install` with a file that is not a package |
| **Expected** | the first install exits 0 and names where it installed; the second exits 0 saying the version is already there and that nothing was done; the list names the application, its version and `complete`; the show names the version, `unsigned` and the payload's digest; the file that is not a package exits 2 |

## yoke:the-manager-command-line.05 — `yoke instance` creates, lists, shows and re-points instances

| Field | Value |
| --- | --- |
| **Cites** | specs/15.67 · specs/15.68 · arch/10-manager/01 §`yoke instance` — records · arch/10-manager/07 §Creating an instance, and naming it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a configuration naming an install root holding `com.yoke.station` at `2.1.0` and `2.2.0`, the account's state directory empty, and a values file |
| **Action** | run `yoke instance create com.yoke.station --name bench-a --params <file>`, then `yoke instance list`, `yoke instance show bench-a`, `yoke instance set-param bench-a setup.retries 5`, `yoke instance set-version bench-a 2.2.0`; then `yoke instance create` of an application not installed |
| **Expected** | each exits 0: the create names the instance and its version, the list names `bench-a`, its application, its version and that it is stopped, the show prints the record with its values; after the last two the record holds the new value and `2.2.0`; the create of an application not installed exits 2 |

## yoke:the-manager-command-line.06 — `yoke instance` copies and restores state, and shows what it occupies

| Field | Value |
| --- | --- |
| **Cites** | specs/15.62 · specs/15.65 · specs/15.66 · arch/10-manager/01 §`yoke instance` — records · arch/10-manager/07 §Copying an instance's state · arch/10-manager/07 §Reading an instance |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a stopped instance `bench-a` whose state holds a log store, and a stopped instance `bench-b` |
| **Action** | run `yoke instance show bench-a`, `yoke instance copy bench-a <path>`, then `yoke instance restore <path> bench-b` |
| **Expected** | each exits 0; the show reports what the log store occupies; the copy names where it wrote; after the restore `bench-b`'s state holds the log store |

## yoke:the-manager-command-line.07 — `yoke app` removes bundles and reclaims what nothing points at

| Field | Value |
| --- | --- |
| **Cites** | specs/15.55 · specs/15.56 · arch/10-manager/01 §`yoke app` — artifacts · arch/10-manager/06 §Removing · arch/10-manager/06 §Reclaiming |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a configuration naming an install root holding `com.yoke.station` at `2.0.0`, `2.1.0` and `2.2.0`, and an engine that does not answer; an instance `bench-a` pinned to `2.0.0` |
| **Action** | run `yoke app remove com.yoke.station`, then `yoke app prune com.yoke.station`, then `yoke app remove com.yoke.station --version 2.2.0`, then `yoke app prune com.yoke.other` |
| **Expected** | the first exits 2 naming `bench-a`, and removes nothing; the prune exits 0 naming `2.1.0` as removed and saying images were not considered, and leaves `2.0.0`, which `bench-a` points at, and `2.2.0`; the removal of `2.2.0` exits 0 naming it; the prune of an application not installed exits 2 |
