# A Plugin project builds

| | |
| --- | --- |
| **Feature** | `yoke-compose build` in a Plugin project: the file checked, the target architecture refused before anything runs where the file does not list it, the command the file names run by `/bin/sh -c` in its directory with five variables set and nothing else of the build the composer's own, inside the image the file names where it names one, and the executable found and left in `build/<arch>/<program>`; the composer interprets no language |
| **Planning item** | yoke-project/yoke#400 |

## yoke:building-a-plugin.01 — the build runs the file's command with the five variables, and leaves the executable

| Field | Value |
| --- | --- |
| **Cites** | specs/95.31 · specs/95.33 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Plugin project whose file lists `x86_64` and `aarch64` and whose `build.run` is a shell command that records its working directory and the values of `YOKE_OUT`, `YOKE_ARCH`, `YOKE_GOARCH`, `YOKE_RUST_TARGET` and `YOKE_DEB_ARCH` into a file outside the project, then writes an executable script named as `program` into `$YOKE_OUT` |
| **Action** | `build --arch aarch64` in the project; then `build` with no `--arch` |
| **Expected** | both exit 0; the command ran in `build.in`; for `aarch64` the variables were `aarch64`, `arm64`, `aarch64-unknown-linux-gnu` and `arm64`, and `YOKE_OUT` an empty directory; for the workstation's own, `x86_64`, `amd64`, `x86_64-unknown-linux-gnu` and `amd64` on `x86_64`, and those of `aarch64` on `aarch64`; the executable is at `build/aarch64/<program>` and at `build/<the workstation's own>/<program>` beside the file, executable; nothing under `plugin/` other than what the command itself wrote has changed |

## yoke:building-a-plugin.02 — an architecture the file does not list is refused before anything runs

| Field | Value |
| --- | --- |
| **Cites** | specs/95.33 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, its file listing `x86_64` alone; its `build.run` writing a mark file when it runs |
| **Action** | `build --arch aarch64`; then `build` with a file the validator refuses |
| **Expected** | both exit non-zero; the first names `aarch64` and the architectures the file lists, the second the validator's finding; no mark file exists and `build/` was not created |

## yoke:building-a-plugin.03 — a command that fails, or that leaves no executable, fails the build

| Field | Value |
| --- | --- |
| **Cites** | specs/95.33 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, with in turn a `build.run` that prints a line and exits 3; one that exits 0 and writes nothing; one that writes the program's file without the executable bit |
| **Action** | `build` each |
| **Expected** | each exits non-zero: the first with the command's exit status and its output passed through; the second naming `$YOKE_OUT/<program>` as absent; the third naming it as not executable; in no case is anything left in `build/<arch>/` |

## yoke:building-a-plugin.04 — the composer sets the five variables and nothing else of the build

| Field | Value |
| --- | --- |
| **Cites** | specs/95.33 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, whose command records its whole environment; the composer invoked with an environment holding `PATH`, `HOME` and a variable `PROBE=1` |
| **Action** | `build` |
| **Expected** | the build's environment is the invoker's with the five variables added; no other variable is added or removed, no language is named in what the composer ran, and the command ran as `/bin/sh -c` with the file's `build.run` verbatim |

## yoke:building-a-plugin.05 — a build that names an image runs inside it, the project at its own path, as the invoker

| Field | Value |
| --- | --- |
| **Cites** | specs/95.44 · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a container engine standing in for the workstation's, named by `CONTAINER_ENGINE`, which records what it is asked and runs the command it is handed on the host; the project of `.01`, its file's `build.container` an image pinned by digest, its command recording `id -u`, its working directory and the five variables, and writing the program; an image reference without a digest |
| **Action** | `build`; then `build` with the reference without a digest |
| **Expected** | the first exits 0; the engine was asked to run the image, removed afterwards, with the project's root and the output directory each mounted at its own path, the working directory `build.in` at the path it has on the host, the invoking user's identifier, the five variables, and `/bin/sh -c` with `build.run` verbatim; the executable is at `build/<arch>/<program>` on the host and owned by the invoker; the second is refused by the validator, naming `build.container`, and nothing ran |
