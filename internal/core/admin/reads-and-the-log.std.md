# Reads, and the log

| | |
| --- | --- |
| **Feature** | `read`, one record per readable subject — the six kinds of the event model — and a listing where no identity is named; a unit's observed state beside its plugin's record, a plugin's declared, authorized and observed groups side by side; and the log store read two ways, `log.query` by pages of 500 with a cursor that is an entry's sequence and resumes past what retention removed, and `log.follow` pushing entries as they are written, bounded at 256 and told where it fell behind |
| **Planning item** | yoke-project/yoke#95 |

## yoke:reads-and-the-log.01 — a read names a kind, lists it where no identity is named, and refuses what is not there

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.37 · specs/60.38 · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two units declared, `acquire` and `archive` |
| **Action** | read the kind `unit`; the unit `acquire`; the unit `nobody`; the kind `stream`; the kind `channel` |
| **Expected** | the kind answers both records, in the order of their identities; `acquire` answers its record alone; `nobody` is refused with `subject.unknown`, naming the kind and the identity; `stream` with `operation.malformed`; `channel` answers no record, there being no channel before 0.3 |

## yoke:reads-and-the-log.02 — a unit's record is its declaration and what is observed, with its plugin's beside it

| Field | Value |
| --- | --- |
| **Cites** | specs/60.42 · arch/60-administrative-surface/05 §Which subjects are readable · arch/60-administrative-surface/05 §Authorised and observed, in one answer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the unit `acquire` of the plugin `com.example.station`, `Running` in its life 3 since a known moment, having reported 80 with the line `warm` |
| **Action** | read it |
| **Expected** | its declared group names it, the kind `plugin`, the backend `host` and the plugin; its observed group `Running`, life 3, the moment and the condition 80 with `warm`; the plugin's record is beside it |

## yoke:reads-and-the-log.03 — a plugin's record marks what was declared, what is authorized and what is observed

| Field | Value |
| --- | --- |
| **Cites** | specs/60.42 · arch/60-administrative-surface/05 §Authorised and observed, in one answer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `com.example.station` declared at protocol 1 and a digest, its last registration saying `0.0.1`, `go` and an SDK line; granted `stream.data.publish`; its Manifest present, composed, and its unit `acquire` running |
| **Action** | read it |
| **Expected** | declared carries the identity, the protocol, the digest, the version, the language and the SDK line; authorized that it is enabled, the grant and the credential mode `bootstrap`; observed that its Manifest is present, that it is composed, and `acquire` as running |

## yoke:reads-and-the-log.04 — the instance, its documents and its connections are read like any subject

| Field | Value |
| --- | --- |
| **Cites** | specs/60.38 · specs/60.39 · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the instance `bench`, ready; a document it read; one shell connection open |
| **Action** | read the kinds `instance`, `document` and `connection` |
| **Expected** | the instance's record names it and says it is ready; the document's its path, what it resolved to and its digest; the connection's its identity, the projection `shell`, the actor and when it opened |

## yoke:reads-and-the-log.05 — log.query answers a page of 500 by a cursor that is an entry's sequence, and resumes past what was removed

| Field | Value |
| --- | --- |
| **Cites** | specs/60.43 · arch/60-administrative-surface/05 §The log store, queried and followed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | 600 entries of `acquire`'s life 1 at severity 10, then 5 of life 2 at severity 50, and 3 of `archive` |
| **Action** | query `acquire` from the beginning; again from the cursor answered; `acquire` at 50 or above; `acquire`'s life 2; then remove the entry a cursor names and query from it |
| **Expected** | the first page holds 500 entries of `acquire` in order, and the cursor of the last; the second the remaining 105; the floor answers the 5; the life answers the 5; the removed cursor answers the entries after it and says it resumed |

## yoke:reads-and-the-log.06 — log.follow pushes entries as they are written, and says where it fell behind

| Field | Value |
| --- | --- |
| **Cites** | specs/60.43 · arch/60-administrative-surface/05 §The log store, queried and followed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a log store holding entries already |
| **Action** | follow `acquire`; append one entry of `acquire` and one of `archive`; then, reading nothing, append 600 entries of `acquire` of 1 KiB each; then read |
| **Expected** | the follow does not answer what was there before it began; it pushes the one entry of `acquire` and not `archive`'s; after the 600, it pushes what its queue of 256 held and then that it fell behind at a sequence, every entry it pushed being at or before it, and a query from that sequence answers the rest — nothing lost and nothing twice |

## yoke:reads-and-the-log.07 — a read, a query and a follow leave no trace

| Field | Value |
| --- | --- |
| **Cites** | arch/60-administrative-surface/03 §Where the identity lands |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core with a log store |
| **Action** | read the kind `unit`; query the log |
| **Expected** | the log store holds no entry more, and nothing was published |

## yoke:reads-and-the-log.08 — through the Core, the instance, a plugin and the log are read

| Field | Value |
| --- | --- |
| **Cites** | specs/60.36 · specs/60.43 · arch/60-administrative-surface/05 §Which subjects are readable |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and started in the service form, with one Manifest in its Plugin directory |
| **Action** | read the instance; read the plugin; query the log |
| **Expected** | the instance is ready and in the service form; the plugin's Manifest is present and it is enabled; the log holds `instance.ready` |
