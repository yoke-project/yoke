# The moments

| | |
| --- | --- |
| **Feature** | the gate's host checks at the moments that had no caller: installing, creating or re-pointing an instance, and every start — the descriptor's architecture and Core range against this host, each component's digest, a Plugin's Manifest against the digest a unit pins, and the launching account's runtime directory |
| **Planning item** | yoke-project/yoke#237 |

## yoke:the-moments.01 — a descriptor for another architecture or another Core is refused wherever the host is in hand, and never while composing

| Field | Value |
| --- | --- |
| **Cites** | specs/13.24 · specs/15.22 · arch/15-gate/06 §The moments · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor written for `riscv64` and Core `>=99.0`, its one component present with its digest; a host of another architecture executing Core `2.0.0` |
| **Action** | check it at installing, at creating and at starting with that host; then while composing |
| **Expected** | the first three give `arch.mismatch` and `core.range`, each naming both values; composing gives neither |

## yoke:the-moments.02 — a component and a Manifest are checked against the digests the descriptor states

| Field | Value |
| --- | --- |
| **Cites** | specs/15.25 · specs/80.39 · arch/15-gate/06 §Phase 4 — host facts · arch/15-gate/06 §The identifying code, and what computing it costs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor whose oneshot names a component whose bytes differ from its stated digest, and whose plugin unit pins a `manifest_digest` the Manifest in the bundle does not have |
| **Action** | check it at creating with a host |
| **Expected** | `component.digest` naming the computed and the stated digests, and `plugin.manifest.digest` naming the Manifest |

## yoke:the-moments.03 — an account with no runtime directory is refused when an instance is made or started, naming what fixes it

| Field | Value |
| --- | --- |
| **Cites** | specs/15.27 · specs/25.49 · arch/30-core/03 §Cleanup belongs to the next start · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor that passes; a host whose account's runtime directory is unset, then one naming a directory that does not exist |
| **Action** | check it at creating and at starting with each; then at installing |
| **Expected** | creating and starting give `runtime.dir.missing`, naming `loginctl enable-linger`; installing gives nothing of it |

## yoke:the-moments.04 — an instance of a version whose installed component was changed is refused at creating

| Field | Value |
| --- | --- |
| **Cites** | specs/15.17 · specs/15.18 · specs/15.25 · specs/15.45 · specs/80.19 · specs/80.37 · arch/10-manager/07 §Creating an instance, and naming it · arch/15-gate/06 §The moments |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `com.yoke.station` `2.1.0` installed and then one of its components overwritten |
| **Action** | run `yoke instance create com.yoke.station` |
| **Expected** | it exits 2 naming `component.digest`, and no record is written |

## yoke:the-moments.05 — a routable channel whose transport material is not in the instance's store is refused when it starts, and only then

| Field | Value |
| --- | --- |
| **Cites** | specs/80.15 · arch/80-security/03 §Where the material comes from · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor declaring a routable channel `panel` and a local channel `local`; a state directory holding no `tls/`, then `tls/panel.crt` alone, then `tls/panel.crt` and `tls/panel.key` with the key unreadable, then both readable |
| **Action** | check it at installing, at creating and at starting, each time |
| **Expected** | at starting, `channel.tls.missing` naming `panel` and the path, for each of the first three, and nothing about `local`; nothing for the fourth; nothing at installing or at creating |

## yoke:the-moments.06 — a routable channel on a host whose authentication service cannot be asked is refused when it starts

| Field | Value |
| --- | --- |
| **Cites** | specs/80.55 · arch/15-gate/06 §Phase 4 — host facts · arch/80-security/04 §The host's answer about a person |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor declaring a routable channel `panel` whose caller is authenticated, its material in the store; the host's answer about its authentication service supplied as a failure, then as a success; then a descriptor declaring no routable channel, with the failure |
| **Action** | check each at installing, at creating and at starting |
| **Expected** | at starting with the failure, `auth.unavailable` naming `panel` and why; nothing of it with the success, nor for the descriptor with no routable channel, nor at installing or at creating |
