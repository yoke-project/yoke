# From a package to a running instance

| | |
| --- | --- |
| **Feature** | flow 1 of the manager: a package the composer built is installed, an instance of it is declared with its values, and started — the guardian gating and executing the Core, the Core launching a unit on the host and a Plugin in a container — and then stopped, with every program built from this tree |
| **Planning item** | yoke-project/yoke#238 |

## yoke:from-a-package-to-a-running-instance.01 — a package becomes a running instance, and stops

| Field | Value |
| --- | --- |
| **Cites** | specs/15.1 · specs/15.46 · specs/15.67 · specs/17.34 · specs/17.40 · arch/10-manager/08 §1 — From a package to a running instance · arch/00-system/06 §2 — The application form, from a record |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke`, `yoke-compose`, `yoke-guardian` and `yoke-core` built from this tree, the guardian on the `PATH` and the Core named by the account's `manager.conf` with the engine's address; an image holding a Plugin built against the Go library, held by the engine; a project declaring `shared.label`, a unit `probe` on the host that writes the label it is handed, and a unit `bench` of kind `plugin` running that image, with the Plugin's Manifest |
| **Action** | build the package with `yoke-compose build`; `yoke app install` it; `yoke instance create` it as `bench-a` with a values file; `yoke instance start bench-a`; once listed observable, read its units; `yoke instance stop bench-a` |
| **Expected** | each verb exits 0; the instance is listed observable; `probe` has completed having written the label, and `bench` is running in a container labelled with the instance; after the stop the instance's claim is free, no container of it runs, and the guardian has exited |
