# Carrying a project forward

| | |
| --- | --- |
| **Feature** | `yoke-compose migrate [--to <model>]` carries the project's descriptor from the model it states to a later one, one step per consecutive model applied in order, `--to` defaulting to the newest model this composer implements; it prints every field changed with its path, its old value and its new one, then every place a step could not carry with what has to be decided there; the result is kept only where no place was left and the gate, against the target model, refuses nothing at or beneath a field a step changed, and otherwise the project is left byte for byte as it was; the sidecars are never touched and no package is built; with one model the chain has no steps, and a descriptor already at the target is left as it is |
| **Planning item** | yoke-project/yoke#284 |

## yoke:migrate.01 — with one model, the chain has no steps and nothing outside it is guessed

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/05 §The verb · arch/20-composer/05 §The field this rests on |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project made by `init`, which states model `1`, the only model this composer implements; a second project whose descriptor states model `0`; a third whose descriptor states no model |
| **Action** | `migrate` the first project; `migrate --to 1`; `migrate --to 2`; `migrate --to 0`; `migrate` the second project; `migrate` the third |
| **Expected** | the first two exit `0` saying the project is already at model `1` and nothing changed; `--to 2` is refused naming `1` as the newest model this composer implements; `--to 0` is refused as a migration backwards; the second project is refused naming `0` as a model no step carries from; the third is refused as stating no model to carry from; every descriptor is byte for byte what it was |

## yoke:migrate.02 — the steps are applied in order, one per consecutive model, and the report names every change

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/05 §The verb · arch/20-composer/05 §What it produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the composer's steps replaced in the test by two, from model `1` to `2` renaming `core` to `engine`, and from `2` to `3` splitting `engine` into `engine.range` and `engine.channel: stable`, with model `3` the newest and a validation that accepts a document at model `3`; a project made by `init` |
| **Action** | `migrate --to 2`, then `migrate` |
| **Expected** | the first leaves the descriptor at model `2` with `engine` holding what `core` held, and reports `core` removed, `engine` written and `model` from `1` to `2`; the second, with no `--to`, carries it to `3`, `engine.range` holding that value and `engine.channel` holding `stable`, each reported with its old and its new value |

## yoke:migrate.03 — the result is validated before it is kept, and what cannot be carried is named with the project unchanged

| Field | Value |
| --- | --- |
| **Cites** | arch/20-composer/05 §The verb · arch/20-composer/05 §What it produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project the gate refuses nothing in, whose descriptor is rewritten to state model `0` and to name its Core range `engine` rather than `core`, beside it a `blocks.yaml` and an `advisories.yaml`; the composer's step from `0` to `1` replaced in the test, in turn, by one renaming `engine` to `core`, by one renaming it to `kore`, and by one leaving `version` and `arch` for a person |
| **Action** | `migrate` with each step in turn |
| **Expected** | the first exits `0`, reports `engine` removed, `core` written and `model` from `0` to `1`, and leaves a descriptor `check` refuses nothing in, with `blocks.yaml` and `advisories.yaml` byte for byte what they were and no package in the project; the second, whose result the gate refuses at `kore`, is refused naming that finding as a defect in the step; the third is refused naming `version` and `arch` and what has to be decided at each; after each of the two refusals the descriptor is byte for byte what it was |
