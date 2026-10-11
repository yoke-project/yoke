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
| **Expected** | each is refused, naming what refused it — the application, the version, its state, its sharing, `parameter.missing`, `parameter.value` — and no instance directory is left behind |

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
