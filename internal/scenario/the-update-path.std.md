# The update path, end to end

| | |
| --- | --- |
| **Feature** | flow 2 of the manager, on the programs built from this tree: a new version installed beside the old and running nothing; the state copied while the old version is stopped; the instance re-pointed and restarted, its migration unit run once and the stamp rewritten; the rollback refused by the stamp, the copy restored and the old version started on the state it wrote; and the superseded version reclaimed once nothing points at it |
| **Planning item** | yoke-project/yoke#315 |

## yoke:the-update-path.01 — an instance is carried to a new version, migrated once, rolled back by its copy, and the old version reclaimed

| Field | Value |
| --- | --- |
| **Cites** | specs/12.26 · specs/12.27 · specs/15.53 · specs/15.62 · specs/15.65 · specs/17.47 · specs/17.48 · specs/17.52 · specs/17.53 · specs/30.27 · specs/30.29 · specs/97.16 · specs/97.17 · specs/97.18 · arch/10-manager/06 §Updating is four operations and not a verb · arch/10-manager/06 §Rolling back · arch/10-manager/06 §Reclaiming |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke`, `yoke-compose`, `yoke-guardian` and `yoke-core` built from this tree, the guardian on the `PATH` and the Core named by the account's `manager.conf`; two packages of one application built by `yoke-compose build` — `1.0.0`, whose `provision` lays out `storage:datasets` as `v1` and whose Plugin `bench` runs on the host after it, and `1.1.0`, declaring `data: ">=1.0"` and adding a unit `migrate` with `migrates_from: "<1.1"` that rewrites the layout as `v2` and counts each run, which `bench` now depends on; `1.0.0` installed, `bench-a` created from it and running |
| **Action** | install `1.1.0`; stop `bench-a` and copy it; `set-version` it to `1.1.0` and restart it; restart it again; stop it and `set-version` it back to `1.0.0`; restore the copy into it, `set-version` it to `1.0.0` and start it; stop it, `set-version` it to `1.1.0`, start it and stop it; `yoke app prune` the application |
| **Expected** | after installing `1.1.0`, `bench-a` is still recorded at `1.0.0`; the copy holds the stamp `1.0.0`; after the restart `migrate` is `Completed`, the layout is `v2`, `migrate` ran once and the stamp is `1.1.0`; after the second restart `migrate` is `Completed` and still ran once; the set-version back to `1.0.0` is refused, naming the data range and `1.1.0`, and the record stays at `1.1.0`; after the restore and the start the layout is `v1`, `migrate` never ran in that state, the stamp is `1.0.0` and `bench` is `Running`; prune removes `1.0.0` and nothing else, and `1.1.0` is still installed |
