# The script that obtains a release

| | |
| --- | --- |
| **Feature** | `yoke-install.sh`, read by `sh` from a release: it obtains that release's files for this machine, checks each against its published digest before keeping any, and on request installs Yoke for the account running it or, as root, for the whole host — stopping at the first thing it cannot do and saying why |
| **Planning item** | yoke-project/yoke#390 |

## yoke:the-install-script.01 — a published copy obtains its own release, checked against the digests it carries

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries · prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a release `0.1.0` served from a directory, the script carrying `0.1.0` and its files' digests, and no manifest anywhere |
| **Action** | run the script with a directory to keep the files in |
| **Expected** | it exits zero and the directory holds the host's and the workstation's files for the architecture asked, byte for byte the released ones — a copy of the newest release needs nothing but itself and the files |

## yoke:the-install-script.02 — another release is checked against the manifest

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same, with a release `0.0.9` served beside it and a manifest listing `0.0.9`'s files |
| **Action** | run the script with `--version 0.0.9` |
| **Expected** | it exits zero, says it checked against the release manifest, and keeps `0.0.9`'s two files |

## yoke:the-install-script.03 — a file that is not the one released is refused, and nothing is kept

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` with its host file's bytes changed after its digest was published |
| **Action** | run the script |
| **Expected** | it exits non-zero naming the file and both digests, and the directory to keep the files in holds nothing — a file is checked before anything is kept, so a refusal leaves no half-release behind |

## yoke:the-install-script.04 — a release nobody published is refused, naming it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The release record |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the manifest of case 02, and a copy of the script taken from the repository, carrying no release |
| **Action** | run the published copy with `--version 0.0.8`, and the repository's copy with no version |
| **Expected** | the first exits non-zero naming `0.0.8` as one the manifest does not list; the second exits non-zero saying it was not published with a release and naming `--version` |

## yoke:the-install-script.05 — a machine no release is built for is refused

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` |
| **Action** | run the script with `--arch riscv64` |
| **Expected** | it exits non-zero naming `riscv64` and the two architectures a release is built for, `amd64` and `arm64`, having downloaded nothing |

## yoke:the-install-script.06 — a different file already kept is replaced only when asked

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the directory to keep the files in already holds a different file under the host file's name |
| **Action** | run the script, then again with `--force` |
| **Expected** | the first exits non-zero naming the file and `--force`, and leaves it as it was; the second exits zero and the file is the released one |

## yoke:the-install-script.07 — installing for an account places the programs, an install root and the guardian's unit

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The application form on disk · arch/10-manager/02 §What is checked, and what is not · arch/25-guardian/02 §How it comes to be running |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0`, an empty home, and a second empty home holding a `manager.conf` of its own |
| **Action** | run the script with `--install account` and a directory for the programs, in each home |
| **Expected** | the programs `yoke`, `yoke-core`, `yoke-guardian`, `yoke-auth` and `yoke-compose` are in that directory and executable; in the first home, `~/yoke/apps` exists with mode `2775` and `manager.conf` names it as `install_root`, and the guardian's user unit names the program in that directory; in the second, `manager.conf` is unchanged — an account's own configuration is never rewritten |

## yoke:the-install-script.08 — installing for the whole host needs root, and says so before downloading

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The service form on disk |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no release served anywhere |
| **Action** | run the script with `--install host`, not as root |
| **Expected** | it exits non-zero saying root is needed and naming `sudo` and `--install account`, not that a file could not be downloaded |

## yoke:the-install-script.09 — a file already kept and released is used, not downloaded again

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` obtained once into a directory, and then served from nowhere |
| **Action** | run the script again with the same directory and `--install account` |
| **Expected** | it exits zero, says it is using each file already kept, and installs — obtaining and then installing downloads each file once |

## yoke:the-install-script.10 — a directory carried to a machine with no network installs it

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/95 §The built binaries · arch/00-system/02 §The application form on disk |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory holding the published script and the host's file of `0.1.0` alone, and no release and no manifest served anywhere |
| **Action** | run the script from that directory with `--dir` naming it, `--runtime-only` and `--install account` |
| **Expected** | it exits zero and the four programs a host runs are installed, and no composer — a device reached by copying a directory needs nothing from the network |

## yoke:the-install-script.11 — an earlier installation is replaced only when upgrading, and its configuration is kept

| Field | Value |
| --- | --- |
| **Cites** | arch/40-state/01 §The schema is stamped in the file · arch/10-manager/02 §What is checked, and what is not |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` installed for an account, and a release `0.2.0` served beside it |
| **Action** | run `0.2.0`'s script with `--install account`, then again with `--upgrade` |
| **Expected** | the first exits non-zero naming a program that differs and `--upgrade`, and leaves `0.1.0`'s programs; the second exits zero, the programs are `0.2.0`'s, it says to start again what was stopped, and `manager.conf` is unchanged — an installation is never replaced by a run that did not say it was replacing one |

## yoke:the-install-script.12 — nothing is upgraded or removed while a Core of the scope runs

| Field | Value |
| --- | --- |
| **Cites** | arch/40-state/01 §The schema is stamped in the file |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` installed for an account, and a process of that account named `yoke-core` running |
| **Action** | run the script with `--install account --upgrade`, then with `--uninstall account` |
| **Expected** | both exit non-zero saying an instance is running and naming `yoke instance stop`, before anything is downloaded or removed, and the programs are as they were — a Core whose executable changes under it is a Core whose next start is a different program |

## yoke:the-install-script.13 — removing from an account keeps its data, and purging refuses while data remains

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The application form on disk |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the release `0.1.0` installed for an account, with an instance's directory in its state and an application in its install root |
| **Action** | run the script with `--uninstall account --purge`; then with `--uninstall account`; then, the instance and the application gone, with `--uninstall account --purge` |
| **Expected** | the first exits non-zero naming the instances and `yoke instance remove`, and removes nothing; the second removes the programs and the guardian's unit and keeps `manager.conf`, the install root and the state; the third removes `manager.conf`'s directory, the install root and the state — an instance and an application are removed by their own verbs, never by a script that cannot know what they hold |

## yoke:the-install-script.14 — removing from the host needs root

| Field | Value |
| --- | --- |
| **Cites** | arch/00-system/02 §The service form on disk |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no release served anywhere |
| **Action** | run the script with `--uninstall host`, not as root |
| **Expected** | it exits non-zero saying root is needed and naming `sudo` |
