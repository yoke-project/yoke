# The remove verb

| | |
| --- | --- |
| **Feature** | `yoke-compose remove <path>`, the inverse of `set`: the node at the path a finding carries and everything beneath it taken away, kept only where the gate refuses nothing it did not already refuse, and otherwise refused with the gate's own finding and the document left byte for byte as it was; a path that names nothing refused as one; a parameter group named after a unit or a channel going with the last of the two; a unit placed from a block losing its line in `blocks.yaml`; a waiver at a removed place left and marked stale; no file beside the descriptor deleted |
| **Planning item** | yoke-project/yoke#318 |

## yoke:the-remove-verb.01 — `remove` takes away the node at a path and everything beneath it

| Field | Value |
| --- | --- |
| **Cites** | specs/95.30 · arch/20-composer/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project holding two units `setup` and `probe`, a parameter `shared.site`, a channel `panel`, and a `policy.startup_window` on `setup`; a composition document holding a unit |
| **Action** | remove `units.setup.policy.startup_window`, `units.probe`, `parameters.shared.site` and `channels.panel` from the project, and the unit from the composition document by `--file` |
| **Expected** | each exits 0 and says what it removed; each node and everything beneath it is gone, an emptied `policy` with it, and everything else in each document is as it was |

## yoke:the-remove-verb.02 — a removal the gate would refuse is not made, and a path naming nothing is refused as one

| Field | Value |
| --- | --- |
| **Cites** | specs/95.30 · arch/20-composer/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project whose unit `probe` depends on `setup`, whose `setup` takes `${site}` as an argument, with a parameter `shared.site` |
| **Action** | remove `units.setup`; remove `parameters.shared.site`; remove `version`; remove `units.nothing` |
| **Expected** | each exits 1 and leaves the descriptor byte for byte as it was; the first three name the gate's finding the removal would have caused; the fourth says that nothing is at `units.nothing` |

## yoke:the-remove-verb.03 — a placed block is taken away with its group and its line, a waiver at a removed place stays stale, and no file is deleted

| Field | Value |
| --- | --- |
| **Cites** | specs/95.30 · arch/20-composer/01 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a project with a block placed as `acquire-2`, recorded in `blocks.yaml` beside another placement, and a oneshot `store` keeping storage while no data range is stated, its advisory waived in `advisories.yaml`; a Manifest and the programs under `components/` |
| **Action** | remove `channels.acquire-2`, then `units.acquire-2`; remove `units.store`; list the waivers |
| **Expected** | the group `acquire-2` went with the unit and `acquire-1`'s is untouched; `blocks.yaml` no longer holds the placement as `acquire-2` and still holds the other; the waiver is still in `advisories.yaml` and `waivers` marks it stale; every program under `components/` and the Manifest are still there |
