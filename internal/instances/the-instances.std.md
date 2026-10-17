# Instances, as the manager makes them

| | |
| --- | --- |
| **Feature** | creating an instance from an installed version: the version chosen when none is named, the values a site supplies checked against the declarations before they are recorded, a name supplied or generated and unique within the account, and the descriptor's digest written with the pin; re-pointing an instance at another version under the data range its state allows; and replacing one value, checked as at creation |
| **Planning item** | yoke-project/yoke#228 |

## yoke:the-instances.01 — an instance is created pinned to the newest complete release, its values recorded with the digest of its descriptor

| Field | Value |
| --- | --- |
| **Cites** | specs/15.67 · specs/17.5 · specs/17.6 · arch/10-manager/07 §Creating an instance, and naming it · arch/10-manager/07 §The values a site supplies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | versions `2.0.0` and `2.1.0` complete, `2.2.0-rc.1` complete and `2.3.0` incomplete; a values file in TOML supplying the one required parameter |
| **Action** | create `bench-a` naming no version |
| **Expected** | the record is pinned to `2.1.0`, names this account its owner, is stopped and not autostarted, carries the supplied value and the defaults, and its `descriptor_digest` is the digest of `2.1.0`'s `yoke.yaml` |

## yoke:the-instances.02 — a name is generated where none is given, and is unique within the account

| Field | Value |
| --- | --- |
| **Cites** | specs/15.67 · specs/17.18 · arch/10-manager/07 §Creating an instance, and naming it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an installed application |
| **Action** | create three instances naming none |
| **Expected** | each is created under a name of its own, reported, and none is created twice |

## yoke:the-instances.03 — an instance is refused where it cannot be made what it declares

| Field | Value |
| --- | --- |
| **Cites** | specs/15.67 · arch/10-manager/07 §Creating an instance, and naming it · arch/10-manager/04 §The install record |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an application not installed; a version absent; one incomplete; one withdrawn; one not shared and installed by another account; values missing the required parameter, and one failing its declaration |
| **Action** | create an instance of each |
| **Expected** | each is refused, naming what refused it — the application, the version, its state, its sharing, `parameter.missing`, `parameter.value` — in a sentence that agrees with it, *the values supplied are refused*; and no instance directory is left behind |

## yoke:the-instances.04 — re-pointing writes the pin and the digest together, and the state's stamp must fall in the data range

| Field | Value |
| --- | --- |
| **Cites** | specs/17.6 · arch/10-manager/06 §Rolling back · arch/10-manager/07 §Creating an instance, and naming it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance pinned to `2.1.0`, whose state the version `2.1.0` last wrote; `2.0.0` installed declaring `data: ">=1.0 <2.1"`, and `2.2.0` declaring none; `1.9.0` withdrawn |
| **Action** | set the version to `2.2.0`; then to `2.0.0`; then to `1.9.0` |
| **Expected** | the first re-points the record and rewrites its digest; the second is refused naming `2.0.0`'s data range and the `2.1.0` that wrote the state; the third is refused as withdrawn |

## yoke:the-instances.05 — replacing one value is checked as at creation

| Field | Value |
| --- | --- |
| **Cites** | specs/15.67 · arch/10-manager/07 §The values a site supplies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance with recorded values |
| **Action** | set one value that satisfies its declaration, then one that fails it, then one nobody declared |
| **Expected** | the first replaces that value and nothing else; the other two are refused, naming them, and the record is unchanged |

## yoke:the-instances.06 — a copy is the state whole and where it came from, and only a stopped instance is copied

| Field | Value |
| --- | --- |
| **Cites** | specs/15.62 · specs/15.64 · arch/10-manager/07 §Copying an instance's state |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance whose state holds both stores, the version stamp, a secret and a storage need's directory; another process holding its claim, then none |
| **Action** | copy it while the claim is held; then copy it; then copy it again to the same path |
| **Expected** | the first is refused as running and writes nothing; the second writes `state/`, every file the state held with its mode, and `origin` naming the instance; the third is refused, naming the path that exists |

## yoke:the-instances.07 — a restore into the same instance keeps the Registry and the secrets, into another strips them

| Field | Value |
| --- | --- |
| **Cites** | specs/15.63 · specs/15.64 · specs/15.65 · specs/80.52 · arch/10-manager/07 §Copying an instance's state |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a copy of `bench-a`; `bench-a` and `bench-b` stopped, `bench-c` with its claim held by another process |
| **Action** | restore the copy into `bench-a`, into `bench-b`, and into `bench-c` |
| **Expected** | `bench-a`'s state is the copy's, Registry, secrets and stamp included; `bench-b`'s is the copy's without the Registry and without secrets, the stamp kept; `bench-c` is refused as running and its state is untouched |

## yoke:the-instances.08 — reading an instance reports what its state occupies, per store and per storage need

| Field | Value |
| --- | --- |
| **Cites** | specs/15.66 · arch/10-manager/07 §Reading an instance |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance whose state holds a Registry of 3 bytes, a log store of 5, and two storage needs of 7 and 11 |
| **Action** | ask what its state occupies |
| **Expected** | the Registry 3, the log store 5, `storage/datasets` 7 and `storage/cache` 11 — read from the filesystem, nothing written |
