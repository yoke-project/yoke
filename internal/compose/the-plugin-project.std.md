# The Plugin project's file

| | |
| --- | --- |
| **Feature** | `yoke-plugin.yaml`, the file at the root of a Plugin project that every step reads: model `1`, the Plugin's identity, its family, its program, where its Manifest is kept, its build — the directory, the command, an image to run it in — and the architectures it is built for; no version, which is the tag; checked by the validator as a descriptor is, each refusal naming the field |
| **Planning item** | yoke-project/yoke#399 |

## yoke:the-plugin-project.01 — a file the template would write is accepted, with its Manifest

| Field | Value |
| --- | --- |
| **Cites** | specs/95.32 · arch/20-composer/07 §The tree · arch/20-composer/07 §`yoke-plugin.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory holding `yoke-plugin.yaml` with every field of the model — `plugin: 1`, `id: com.example.greeter`, `language: go`, `program: greeter`, `manifest: plugin/com.example.greeter.yaml`, `build.in: plugin/`, a `build.run`, `architectures: [x86_64, aarch64]` — and no `build.container`; `plugin/com.example.greeter.yaml`, a Manifest the gate accepts, of `id` `com.example.greeter`; `try/`, an application project made by `init` |
| **Action** | read and check the project; then the same with `build.container` set to an image reference pinned by digest |
| **Expected** | both are accepted with no finding; what is read carries every field as written, the Manifest read from the path named, and the image reference where it was set; nothing under `try/` is read as part of the Plugin |

## yoke:the-plugin-project.02 — a field missing, or of the wrong kind, is refused by its name

| Field | Value |
| --- | --- |
| **Cites** | specs/95.32 · arch/20-composer/07 §`yoke-plugin.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`; one copy of its file for each required field — `plugin`, `id`, `language`, `program`, `manifest`, `build.in`, `build.run`, `architectures` — with that field removed; one with `plugin: 2`; one with `language: java`; one with `architectures` empty; one with a field the model does not have; one that is not YAML |
| **Action** | check each |
| **Expected** | each is refused, the finding naming the field removed, `plugin` and the model it read, `language` and the families it takes, `architectures`, the unknown field, and the file; nothing is built and nothing is written |

## yoke:the-plugin-project.03 — the file carries no version

| Field | Value |
| --- | --- |
| **Cites** | specs/95.38 · arch/20-composer/07 §`yoke-plugin.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, its file given a field `version: 1.2.0` |
| **Action** | check it |
| **Expected** | it is refused, the finding naming `version` and saying the version is the tag; with the field removed, the same file is accepted |

## yoke:the-plugin-project.04 — the identity is the Manifest's

| Field | Value |
| --- | --- |
| **Cites** | specs/95.32 · arch/20-composer/07 §`yoke-plugin.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, with its file's `id` changed to `com.example.other`; and the project of `.01` with the Manifest the file names missing; and with a Manifest the gate refuses |
| **Action** | check each |
| **Expected** | the first is refused naming `id`, both identities stated; the second naming `manifest` and the path; the third with the gate's finding on the Manifest |

## yoke:the-plugin-project.05 — every path stays inside the project, and the Manifest inside the build's directory

| Field | Value |
| --- | --- |
| **Cites** | specs/95.32 · arch/20-composer/07 §`yoke-plugin.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, with in turn: `build.in: ../elsewhere/`; `build.in` an absolute path; `manifest: ../greeter.yaml`; `manifest: try/manifests/com.example.greeter.yaml`, a Manifest present there but outside `build.in`; `build.in` naming a directory that does not exist; `program: bin/greeter`, a name with a separator |
| **Action** | check each |
| **Expected** | each is refused, naming respectively `build.in`, `build.in`, `manifest`, `manifest` with `build.in`, `build.in` with the directory, and `program` |

## yoke:the-plugin-project.06 — an architecture the build cannot be told is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/95.32 · arch/20-composer/07 §`yoke-plugin.yaml` · arch/20-composer/07 §The build contract |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`, with in turn `architectures: [amd64]`, `[x86_64, riscv64]` and `[x86_64, x86_64]` |
| **Action** | check each |
| **Expected** | each is refused, naming `architectures` and the value — `amd64` with the `uname` name it stands for, `riscv64` as one the build contract has no spelling for, and the repetition |

## yoke:the-plugin-project.07 — a directory is a Plugin project, an application project, or neither

| Field | Value |
| --- | --- |
| **Cites** | specs/95.31 · arch/20-composer/07 §The build contract · arch/20-composer/02 §The working directory |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01`; its `try/`; a directory holding both `yoke-plugin.yaml` and `yoke.yaml`; an empty directory |
| **Action** | ask the composer which kind of project each is |
| **Expected** | a Plugin project, an application project, a refusal naming the two files, and a refusal saying neither file is there |
