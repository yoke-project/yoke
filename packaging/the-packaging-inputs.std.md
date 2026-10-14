# The packaging inputs

| | |
| --- | --- |
| **Feature** | what `yoke` ships for a distribution beside the programs: the default `core.yaml`, and the directory layout with its ownership as systemd reads it — a `tmpfiles.d` file and a `sysusers.d` file |
| **Planning item** | yoke-project/yoke#286 |

## yoke:the-packaging-inputs.01 — the default `core.yaml` states every key at its default

| Field | Value |
| --- | --- |
| **Cites** | arch/30-core/01 §`core.yaml`, in the service form · prj_structure/40 F3 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `packaging/core.yaml` as shipped |
| **Action** | read it as the Core reads its configuration, and read an empty configuration |
| **Expected** | it is accepted, and it states every key the Core reads, each at the value an empty configuration takes |

## yoke:the-packaging-inputs.02 — the layout creates the service form's directories and the install root, with their owners and modes, and nothing else

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The service form on disk · arch/10-manager/04 §Who owns what · prj_structure/97 §`meta-yoke`, and what its recipes do |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `packaging/yoke.tmpfiles` as shipped |
| **Action** | read its lines |
| **Expected** | one `d` line for each of `/etc/yoke`, `/etc/yoke/plugins.d`, `/etc/yoke/deployments`, `/usr/lib/yoke`, `/usr/lib/yoke/plugins` and `/usr/lib/yoke/interfaces` at `0755`, root and root; `/var/lib/yoke` at `0755`, root and root; `/var/lib/yoke/apps` at `2775`, root and `yoke-apps`; and no other line |

## yoke:the-packaging-inputs.03 — the group the install root needs is created with it

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The service form on disk · arch/10-manager/02 §What is checked |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `packaging/yoke.sysusers` as shipped |
| **Action** | read its lines |
| **Expected** | one line, a group `yoke-apps` with an identifier the system allocates; no account is created |
