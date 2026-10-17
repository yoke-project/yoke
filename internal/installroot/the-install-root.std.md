# The install root and the install record

| | |
| --- | --- |
| **Feature** | the shared root bundles are installed under, checked for its properties and never for having been configured; one directory per application and one per version, the version's created exclusively so that two installs of it decide between themselves; the install record beside the bundle, written `incomplete` first; and what is read from the tree rather than stored |
| **Planning item** | yoke-project/yoke#225 |

## yoke:the-install-root.01 — a root is checked for its properties, each refused by name

| Field | Value |
| --- | --- |
| **Cites** | specs/15.40 · specs/15.41 · specs/15.42 · specs/15.74 · arch/10-manager/02 §What is checked, and what is not |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory of this account's group with set-group-id and mode 2775; then a path that is a file, a directory of a group this account is not in, one without set-group-id, one the group may not write, and one others may write |
| **Action** | check each as a root |
| **Expected** | the first passes; each of the others is refused, naming the property it lacks |

## yoke:the-install-root.02 — a version directory is created exclusively, and a second install of it reads the record it finds

| Field | Value |
| --- | --- |
| **Cites** | specs/15.48 · specs/15.50 · arch/10-manager/04 §Two installs at once |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root holding nothing |
| **Action** | begin an installation of `com.yoke.station` `2.1.0`, then begin it again, then complete the first and begin it a third time, then begin `2.2.0` |
| **Expected** | the first creates both directories and writes a record `incomplete`; the second is refused as an installation in progress or interrupted; the third as already present; `2.2.0` proceeds beside it |

## yoke:the-install-root.03 — the record says what the filesystem cannot

| Field | Value |
| --- | --- |
| **Cites** | specs/15.21 · specs/13.23 · arch/10-manager/04 §The install record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an installation begun by this account |
| **Action** | write its record with a key id, sealed, a digest and shared; read it back; then read records with an unknown key and with a state that is none of the three |
| **Expected** | every field reads back as written, `installed_by` the account's name and `installed_at` in UTC; the unknown key and the unknown state are each refused, naming them |

## yoke:the-install-root.04 — directories and the record carry the modes the tree states

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/04 §The tree |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root of mode 2775 and this account's group |
| **Action** | begin an installation |
| **Expected** | the application's and the version's directories are 2775 and in the root's group, and `install.yaml` is 0664 |

## yoke:the-install-root.05 — what is installed is read from the tree, and nothing lists it

| Field | Value |
| --- | --- |
| **Cites** | specs/15.46 · arch/10-manager/04 §What is read from the tree |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a root holding `com.yoke.station` at `2.1.0` complete, `2.10.0` complete, `2.2.0-rc.1` incomplete, and `com.yoke.other` at `1.0.0` |
| **Action** | list what is installed, and ask for the newest version of `com.yoke.station` |
| **Expected** | both applications are listed with their versions ordered as semantic versions and each record's state; the newest complete version is `2.10.0`; who installed each is the owner of its directory; no file in the root lists any of it |

## yoke:the-install-root.06 — a withdrawn version refuses new pins

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/04 §The install record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | records in each of the three states |
| **Action** | ask of each whether a new instance may pin it |
| **Expected** | only a `complete` record may be pinned; `withdrawn` and `incomplete` are refused, naming the state |
