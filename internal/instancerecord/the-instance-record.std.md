# The instance record

| | |
| --- | --- |
| **Feature** | the record that declares an instance: one per instance, beside the state it declares, under the account's state directory; its fields read strictly, its name checked against the directory holding it; created by a directory made exclusively and the record written last; replaced by a temporary file and a rename, two edits serialised by an advisory lock; an ephemeral run's four fields; and the set of records a directory listing |
| **Planning item** | yoke-project/yoke#230 |

## yoke:the-instance-record.01 — a record sits under the account's state directory, one directory per instance

| Field | Value |
| --- | --- |
| **Cites** | specs/17.14 · specs/17.15 · arch/25-guardian/01 §Where it sits |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `XDG_STATE_HOME` set; then unset, with `HOME` set |
| **Action** | ask where the instance `bench-a`'s record is |
| **Expected** | `$XDG_STATE_HOME/yoke/instances/bench-a/instance.yaml`, then `$HOME/.local/state/yoke/instances/bench-a/instance.yaml`; its state directory is `state/` beside it |

## yoke:the-instance-record.02 — a record holds what it declares, read strictly

| Field | Value |
| --- | --- |
| **Cites** | specs/17.5 · specs/17.6 · specs/17.7 · specs/17.8 · specs/17.11 · specs/17.25 · arch/25-guardian/01 §What it holds |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a record with every field of a declared record |
| **Action** | write it and read it back; then read records with an unknown key, a `desired_state` of `paused`, and a `name` that is not its directory's |
| **Expected** | every field reads back as written; the unknown key, the state and the name are each refused, naming what was found |

## yoke:the-instance-record.03 — a name is unique because creating it makes a directory that cannot already exist

| Field | Value |
| --- | --- |
| **Cites** | specs/17.18 · specs/17.19 · arch/25-guardian/01 §Who writes it, and how |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | no instance `bench-a` |
| **Action** | create it; create it again; then make a directory `bench-b` holding no record, and list the instances |
| **Expected** | the first creation writes the directory and the record; the second is refused naming the instance; `bench-b` is listed as holding no valid record and is no instance |

## yoke:the-instance-record.04 — two edits to one record are serialised, and a write replaces the file

| Field | Value |
| --- | --- |
| **Cites** | specs/17.17 · arch/25-guardian/01 §Who writes it, and how |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a record |
| **Action** | run twenty edits at once, each adding one parameter value to it, and read it |
| **Expected** | all twenty values are there, and no temporary file is left in its directory |

## yoke:the-instance-record.05 — an ephemeral record carries its four fields, and only there

| Field | Value |
| --- | --- |
| **Cites** | specs/17.28 · arch/25-guardian/01 §An ephemeral record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an ephemeral record with a source, the verification waived and unlaunched admission permitted; then a declared record carrying `verification`, and an ephemeral one carrying `verification` with no source |
| **Action** | write the first and read it back; read the other two |
| **Expected** | the first reads back with all four fields; each of the other two is refused, naming the field |

## yoke:the-instance-record.06 — the set of records is a directory listing, and what points at a version is read from it

| Field | Value |
| --- | --- |
| **Cites** | specs/17.14 · specs/17.20 · specs/17.21 · arch/10-manager/06 §Removing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | three instances, two pinned to `com.yoke.station` `2.1.0` and one to `2.0.0` |
| **Action** | list the instances, and ask which point at `2.1.0` |
| **Expected** | the three are listed by name; the two pinned to `2.1.0` are named, and nothing else in the directory lists them |
