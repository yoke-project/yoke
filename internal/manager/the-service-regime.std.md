# The service regime

| | |
| --- | --- |
| **Feature** | `yoke service`: `start`, `stop`, `restart` and `status` asking the init system about `yoke.service` and starting nothing; `status` naming a disagreement between the init system and the instance rather than choosing; `check` running the gate over the composition in force; `set-secret` writing into the deployment's own area without displaying the value; `exec`, which the unit names, gating and then executing the Core in one process; and the unit itself |
| **Planning item** | yoke-project/yoke#242 |

## yoke:the-service-regime.01 — `start`, `stop` and `restart` ask the init system and start nothing

| Field | Value |
| --- | --- |
| **Cites** | specs/15.9 · specs/15.16 · arch/10-manager/01 §`yoke service` — the system regime |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `systemctl` the test puts on the `PATH`, recording its arguments, and failing when told to |
| **Action** | run `yoke service start`, `stop` and `restart`; then `start` with the `systemctl` failing |
| **Expected** | each of the three exits 0, having asked `systemctl` for `start`, `stop` and `restart` of `yoke.service` and nothing else; the failing one exits 1 and says what `systemctl` said |

## yoke:the-service-regime.02 — `status` reports the init system and the instance beside each other, and names a disagreement

| Field | Value |
| --- | --- |
| **Cites** | specs/15.9 · specs/15.77 · arch/10-manager/01 §What it reports |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` whose runtime directory holds no instance, and an account holding an instance record; a `systemctl` saying `yoke.service` is active, then one saying it is inactive |
| **Action** | run `yoke service status` with each |
| **Expected** | the first says the init system reports it active, that the instance does not answer at the deployment's own operator socket even though the account holds an instance record, and names the disagreement; the second reports both as not running and names none; both exit 0 |

## yoke:the-service-regime.03 — `check` runs the gate over the composition in force, and reports every refusal

| Field | Value |
| --- | --- |
| **Cites** | specs/15.36 · arch/15-gate/01 §Who invokes it, and when · arch/15-gate/06 §The phases |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml`; a composition in force with two units whose executables are not there; a second composition with nothing to refuse |
| **Action** | run `yoke service check`; then with `--composition` naming the second |
| **Expected** | the first exits 1 naming `component.missing` for both units; the second exits 0 saying it passes |

## yoke:the-service-regime.04 — `set-secret` writes into the deployment's own area and never displays the value

| Field | Value |
| --- | --- |
| **Cites** | specs/15.57 · specs/15.59 · arch/10-manager/07 §Supplying a secret |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a `core.yaml` whose state directory is the test's own; a value on the standard input |
| **Action** | run `yoke service set-secret token`; then `set-secret ../token` |
| **Expected** | the first exits 0 saying the secret is present, without the value; `<state_dir>/secrets/token` holds the value, mode `0600`, in a directory of mode `0700`; the second exits 2 and writes nothing |

## yoke:the-service-regime.05 — `exec` gates the composition and then executes the Core in its own place

| Field | Value |
| --- | --- |
| **Cites** | specs/15.16 · arch/00-system/06 §1 — The service form, from boot · arch/10-manager/01 §`yoke service` — the system regime |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke` built; a `yoke-core` the test puts on the `PATH`, writing its process and its arguments; the two compositions of `.03` |
| **Action** | run `yoke service exec --composition` with the refused one; then with the one that passes |
| **Expected** | the first exits 1 naming the refusals, and no Core was executed; the second is the Core, in the same process, executed with `--composition` naming it |

## yoke:the-service-regime.06 — the shipped unit, under systemd, runs the deployment the composition declares

| Field | Value |
| --- | --- |
| **Cites** | specs/15.16 · arch/00-system/06 §1 — The service form, from boot · prj_structure/97 §`meta-yoke`, and what its recipes do |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke` and `yoke-core` built; `packaging/yoke.service` installed for the account's own systemd, a drop-in naming the built `yoke`, a `core.yaml` and a composition of the test's own; a `systemctl` on the `PATH` that is the account's own |
| **Action** | run `yoke service start`; `yoke service status`; `yoke service stop` |
| **Expected** | the start exits 0 and the instance becomes ready under the unit; the status says the unit is active and the instance answers, naming no disagreement; after the stop the unit is inactive and the instance's claim is free |

## yoke:the-service-regime.07 — the shipped unit restarts after a fault and never after a refusal

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/01 §What it reports · arch/00-system/06 §1 — The service form, from boot |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `packaging/yoke.service` as shipped |
| **Action** | read its `[Service]` section |
| **Expected** | `Restart=on-failure`, and `RestartPreventExitStatus=` names exactly the status `yoke` exits with on a refusal, `1` |
