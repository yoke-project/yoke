# The Core process, its configuration, its name and its claim

| | |
| --- | --- |
| **Feature** | the Core as a process in the service form: what it is executed with, `core.yaml` read once with its defaults and its overrides, the name every path derives from, and the claim that decides which process serves an instance |
| **Planning item** | yoke-project/yoke#7 |

## yoke:the-core-process.01 — a key core.yaml leaves out takes its default

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §core.yaml, in the service form · specs/20.6 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an empty `core.yaml`, and one setting only `state_dir` |
| **Action** | read each |
| **Expected** | every key the file leaves out has its default — `/var/lib/yoke`, `/run/yoke`, `/etc/yoke/plugins.d`, `/usr/lib/yoke/plugins`, `30s`, `unix:///run/podman/podman.sock`, `info` — and the one it sets has the value it sets |

## yoke:the-core-process.02 — an unknown key, at any level, is refused by name

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §core.yaml, in the service form · specs/20.7 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` with an unknown key at the top, and another with one under `plugins` |
| **Action** | read each |
| **Expected** | each is refused, and the refusal names the key — a key nothing reads is one an operator believes works |

## yoke:the-core-process.03 — a value is typed by the key it sets, never by its notation

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §core.yaml, in the service form · specs/20.7 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` setting `state_dir: no`, another setting `plugins.scan_interval: 30`, and another setting it to `soon` |
| **Action** | read each |
| **Expected** | the first is read with `state_dir` the string `no`; the other two are refused, naming `plugins.scan_interval` — an interval is a duration, and a number or a word is not one |

## yoke:the-core-process.04 — the environment overrides one value, validated as the file's are

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §The override chain · specs/20.7 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` setting `state_dir` and `plugins.scan_interval`, and the environment setting `YOKE_STATE_DIR` and `YOKE_PLUGINS_SCAN_INTERVAL`; then the environment setting `YOKE_PLUGINS_SCAN_INTERVAL=soon` |
| **Action** | read the configuration with each environment |
| **Expected** | with the first, both values are the environment's; with the second, it is refused, naming the variable — the name of a variable is the key's path, upper-cased and joined by `_`, and a value from the environment passes the same gate |

## yoke:the-core-process.05 — YOKE_CONFIG names the file read in place of the default

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §The override chain |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` in a directory of its own, and `YOKE_CONFIG` naming it; then `YOKE_CONFIG` naming a file that does not exist |
| **Action** | load the configuration with each |
| **Expected** | the first is that file's values; the second is refused, naming the path — a named file that cannot be read is never replaced by defaults |

## yoke:the-core-process.06 — the composition in force is chosen by the chain, the flag last

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The service form on disk · specs/20.9 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | neither `YOKE_COMPOSITION` nor `--composition`; then `YOKE_COMPOSITION` alone; then both |
| **Action** | resolve the composition in force for each |
| **Expected** | `/etc/yoke/deployment.yaml`, then the variable's path, then the flag's — each link overrides the one before it, and nothing inside `core.yaml` names a composition |

## yoke:the-core-process.07 — a name that could choose where the instance writes is refused, and a valid one is never changed

| Field | Value |
| --- | --- |
| **Cites** | specs/25.9 · specs/25.39 · arch/30-core/02 §The name |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the names `bench-a`, `Bench A`, `a/b`, `..`, `.`, a name holding a null byte, and the empty name |
| **Action** | validate each, and derive the paths of each valid one |
| **Expected** | `bench-a` and `Bench A` are accepted and appear in their paths exactly as given; the others are refused — a separator, a null byte or a name that is a directory's own reference would let whoever supplies it choose where the instance writes |

## yoke:the-core-process.08 — every path is derived from the name, and the name appears once

| Field | Value |
| --- | --- |
| **Cites** | specs/25.3 · specs/25.8 · specs/20.4 · arch/30-core/01 §The application form has no equivalent, and needs none |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the service form with `runtime_dir` and `state_dir` set; the application form with the name `bench-a` and `XDG_RUNTIME_DIR` and `XDG_STATE_HOME` set |
| **Action** | derive the paths of each |
| **Expected** | the service form's root is `runtime_dir`, its state is `state_dir` and its name is `yoke`; the application form's root is `$XDG_RUNTIME_DIR/yoke/bench-a` and its state `$XDG_STATE_HOME/yoke/instances/bench-a/state`; in both the claim is `<root>/instance.lock` — the address of an instance is computable from its name with no lookup |

## yoke:the-core-process.09 — a second process claiming a held instance is refused, naming the holder

| Field | Value |
| --- | --- |
| **Cites** | specs/25.11 · specs/25.12 · arch/30-core/02 §The claim |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root claimed by another process |
| **Action** | claim it |
| **Expected** | the claim fails, and the failure says the instance is already running as that process's identifier — the refusal is the report, and it is inspectable because the lock can be asked who holds it |

## yoke:the-core-process.10 — the claim is released when its holder dies by any means

| Field | Value |
| --- | --- |
| **Cites** | specs/25.12 · arch/30-core/02 §The claim |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root claimed by another process, which is then killed with a signal that runs no cleanup |
| **Action** | claim it once the holder is gone |
| **Expected** | the claim succeeds, with the lock file left where the holder left it — a claim that outlived its holder would make every crash a manual recovery |

## yoke:the-core-process.11 — a second claim inside the holding process neither succeeds nor releases the first

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/02 §The claim |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root this process has claimed |
| **Action** | claim it again from this process; then claim it from another process |
| **Expected** | the second claim in this process is refused, and the other process is still refused — a record lock is released when any descriptor on its file closes, so the file is opened once and never again |

## ~~yoke:the-core-process.12 — the Core serves one instance at a time, and says who does~~ — moved to `cmd/yoke-core/the-core-binary.std.md` as `yoke:the-core-binary.01`, beside its test

## ~~yoke:the-core-process.13 — a configuration that does not pass prevents startup~~ — moved to `cmd/yoke-core/the-core-binary.std.md` as `yoke:the-core-binary.02`, beside its test

## yoke:the-core-process.14 — log.level is one of four levels, and nothing else

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §core.yaml, in the service form · specs/20.7 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` setting `log.level` to each of `debug`, `info`, `warn` and `error`, and one setting it to `verbose` |
| **Action** | read each |
| **Expected** | the four are read as written, and `verbose` is refused naming `log.level` — a level the logger would interpret on its own is a value validated nowhere |
