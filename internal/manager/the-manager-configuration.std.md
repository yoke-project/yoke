# The manager's configuration

| | |
| --- | --- |
| **Feature** | `$XDG_CONFIG_HOME/yoke/manager.conf`: where bundles are installed, how the engine is reached, which Core to execute and the account's retention ceiling; every value defaulting from the host's conventions, so that a missing file is not an error; an unknown key refused at any level |
| **Planning item** | yoke-project/yoke#224 |

## yoke:the-manager-configuration.01 — the file is read

| Field | Value |
| --- | --- |
| **Cites** | specs/15.72 · specs/15.44 · arch/10-manager/02 §Where it is and what it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file stating `install_root`, `engine`, `core` and a retention ceiling of `bytes` and `age` |
| **Action** | read it |
| **Expected** | each value is the one written; the ceiling holds `bytes` and `age` and no `entries` |

## yoke:the-manager-configuration.02 — a missing file is not an error, and every value takes its default

| Field | Value |
| --- | --- |
| **Cites** | specs/15.73 · arch/10-manager/02 §Where it is and what it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no file at all, `XDG_RUNTIME_DIR` set to `/run/user/1000` |
| **Action** | read the configuration |
| **Expected** | the install root is `/var/lib/yoke/apps`, the engine `unix:///run/user/1000/podman/podman.sock`, the Core `yoke-core`, and there is no ceiling |

## yoke:the-manager-configuration.03 — an unknown key is refused at any level

| Field | Value |
| --- | --- |
| **Cites** | specs/15.43 · arch/10-manager/02 §Where it is and what it is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | files with an unknown key at the top, and with `files` under `retention` |
| **Action** | read each |
| **Expected** | each is refused, naming the key and where it stands |

## yoke:the-manager-configuration.04 — each value is what it says

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/02 §Where it is and what it is · arch/15-gate/05 §The formats |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | files whose `install_root` is relative, whose `retention.bytes` is `20 megabytes`, whose `retention.age` is `a week` and whose `retention.entries` is `many` |
| **Action** | read each |
| **Expected** | each is refused, naming the field and what was written |

## yoke:the-manager-configuration.05 — the file is found under `XDG_CONFIG_HOME`, or under the home directory's `.config`

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/02 §The figures this document sets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file under `$XDG_CONFIG_HOME/yoke/`, then `XDG_CONFIG_HOME` unset and a file under `$HOME/.config/yoke/` |
| **Action** | read the configuration each time |
| **Expected** | each time the file found is the one written there |
