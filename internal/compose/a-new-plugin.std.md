# A Plugin starts from its family's template

| | |
| --- | --- |
| **Feature** | `yoke-compose new plugin <id> --language <family> [--dir <d>] [--program <p>] [--release <v>]` or `[--template <archive>]`: the family's template found through the manifest's cut line for the composer's own release (or the release named), obtained and checked against the digest its line states; `template/` unpacked into the destination with `@YOKE_ID@` and `@YOKE_PROGRAM@` substituted in contents and names and nothing else changed; the Plugin's file checked; `try/` made an application project with `init` and the Plugin taken into it from `..`; a composer that is no release's build refused, naming `--release` and `--template`; `--template` reading a carried archive with no network, its digest printed; on any refusal before the take nothing is left, and a failed take leaves the tree and says the command that repeats it |
| **Planning item** | yoke-project/yoke#403 |

## yoke:a-new-plugin.01 — a release's composer finds its family's template through its cut, and writes the Plugin project

| Field | Value |
| --- | --- |
| **Cites** | specs/95.31 · arch/20-composer/07 §The template · prj_structure/97 §A family's Plugin template · specs/95.46 · specs/95.47 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a composer stating `v0.12.0`; a manifest whose cut line for `v0.12.0` names `yoke-plugin-template-go` at `v0.9.0`, whose publication line for it states the archive's digest and its release; the archive served there, holding `template/` with `yoke-plugin.yaml`, `plugin/@YOKE_ID@.yaml`, a source file naming both placeholders and an `@YOKE_OTHER@` that is none of them, and `try/README.md`; the template's build writing a fixed executable, for this host's architecture |
| **Action** | `new plugin com.example.greeter --language go`, in an empty directory |
| **Expected** | exit 0; `greeter/` holds `yoke-plugin.yaml`, `plugin/com.example.greeter.yaml` and the source with `com.example.greeter` and `greeter` where the placeholders were and `@YOKE_OTHER@` untouched, every other byte as the archive's; `try/yoke.yaml` is an application `com.example.greeter-try` for this host's architecture with Core range `>=0.12.0`; the unit `greeter` is declared there, its executable and Manifest placed, and `plugins.yaml` records the source `..` |

## yoke:a-new-plugin.02 — `--dir` and `--program` name the destination and the program

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template · arch/20-composer/07 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composer, manifest and archive of `.01` |
| **Action** | `new plugin com.example.greeter --language go --dir hello-plugin --program hello` |
| **Expected** | exit 0; the tree is in `hello-plugin/`, its file's `program` is `hello`, and the unit in `try/` is `hello` |

## yoke:a-new-plugin.03 — a composer that is no release's build refuses, and `--release` names one

| Field | Value |
| --- | --- |
| **Cites** | specs/95.31 · arch/20-composer/07 §The template · specs/95.48 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the manifest and archive of `.01`; composers stating a pseudo-version, `v0.12.0+dirty`, and `(devel)` |
| **Action** | `new plugin com.example.greeter --language go` with each; then with the pseudo-version's, `--release 0.12.0` |
| **Expected** | each of the first three exits 1 naming `--release` and `--template`, reads nothing from the network and leaves no `greeter/`; the last writes the tree as `.01` does, the Core range `>=0.12.0` |

## yoke:a-new-plugin.04 — `--template` reads a carried archive, with no network, and prints its digest

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template · prj_structure/97 §A family's Plugin template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the archive of `.01` as a file; a composer that is no release's build; no manifest reachable |
| **Action** | `new plugin com.example.greeter --language go --template <file>` |
| **Expected** | exit 0; the output states the archive's SHA-256 and that it was checked against no record; nothing is read from the network; the tree is written as `.01`'s, `try/`'s Core range the composer's default |

## yoke:a-new-plugin.05 — what the record does not hold, or does not match, is refused and leaves nothing

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template · specs/95.46 · specs/95.47 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composer and archive of `.01`; a manifest with no cut line for `v0.12.0`; one whose cut names no template for `rust`; one whose template line states another digest |
| **Action** | `new plugin com.example.greeter` with `--language go` against the first and third, `--language rust` against the second |
| **Expected** | each exits 1, naming respectively the version with no cut, the family with no template, and the digest that differs; no `greeter/` is left |

## yoke:a-new-plugin.06 — what the command is given is checked before anything is read

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composer and manifest of `.01`; a directory `greeter/` already there |
| **Action** | `new plugin` with an identifier that is not one; with `--language java`; with both `--release` and `--template`; with `--program a/b`; with no `--language`; and `com.example.greeter --language go` beside `greeter/` |
| **Expected** | each exits 1 naming what is wrong, reads nothing from the network, and `greeter/` is left as it was |

## yoke:a-new-plugin.07 — an archive that is not a template is refused, and the destination removed

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | carried archives: one with no `template/`; one with an entry leaving the destination; one whose `try/` holds a `yoke.yaml`; one whose `yoke-plugin.yaml` the validator refuses |
| **Action** | `new plugin com.example.greeter --language go --template <file>` with each |
| **Expected** | each exits 1, naming respectively `template/`, the entry, `try/yoke.yaml` and the file's field; no `greeter/` is left and nothing was written outside it |

## yoke:a-new-plugin.08 — a failed take leaves the tree and says how to repeat it

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/07 §The template |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a carried archive like `.01`'s whose build fails |
| **Action** | `new plugin com.example.greeter --language go --template <file>` |
| **Expected** | exit 1; `greeter/` holds the Plugin project and `try/yoke.yaml`, with no unit declared and no `plugins.yaml`; the output names `add plugin ..` run in `greeter/try` as the command that repeats the take |
