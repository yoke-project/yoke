# The package

| | |
| --- | --- |
| **Feature** | the transport form of an application: a Zip payload with a 512-byte footer appended, `SIG` then `META`, read from the last bytes of the file; the three checks — the CRC over the metadata, the signature over it, the digest of the payload — each reported as itself, in the order that trusts nothing before it is checked; sealing; the comparisons against the host; and producing a package, which refuses a value it cannot carry |
| **Planning item** | yoke-project/yoke#221 |

## yoke:the-package.01 — the footer is 512 bytes at the end of the file, laid out as the format says

| Field | Value |
| --- | --- |
| **Cites** | specs/13.4 · specs/13.5 · specs/13.31 · arch/10-manager/03 §The footer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | metadata with every field set |
| **Action** | produce a package and read its last 512 bytes |
| **Expected** | `SIG` occupies `EOF-512` to `EOF-256` and `META` the last 256 bytes; `META` begins `YOKEPKG1`, format `1`, covered length `252`, every field at its offset, little-endian, text zero-padded, reserved bytes zero, and the CRC-32 over its first 252 bytes in the last four |

## yoke:the-package.02 — a package in clear opens with an ordinary Zip reader

| Field | Value |
| --- | --- |
| **Cites** | specs/13.6 · arch/10-manager/03 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package in clear whose payload holds `yoke.yaml`, `manifests/` and `components/` |
| **Action** | open the whole file with the standard library's Zip reader |
| **Expected** | it lists the payload's entries and reads each, the footer notwithstanding |

## yoke:the-package.03 — a file that is not a package, and one that is damaged, are reported as such and apart from a failure of trust

| Field | Value |
| --- | --- |
| **Cites** | specs/13.7 · specs/13.11 · arch/10-manager/03 §The order · arch/10-manager/03 §Three checks, three different things |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file shorter than 512 bytes, one whose `magic` is not `YOKEPKG1`, one of format `2`, and one whose metadata differs by a byte from what its CRC covers |
| **Action** | read the footer of each |
| **Expected** | the first three are reported as not a package and the last as damaged; none of them is reported as a failure of trust, and nothing past the footer was read |

## yoke:the-package.04 — reserved bytes that are not zero are refused

| Field | Value |
| --- | --- |
| **Cites** | specs/13.5 · arch/10-manager/03 §The footer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a footer whose reserved bytes carry a value, its CRC recomputed |
| **Action** | read it |
| **Expected** | it is refused as a footer this reader cannot read, naming the reserved bytes |

## yoke:the-package.05 — a value that does not fit is refused when producing, never truncated

| Field | Value |
| --- | --- |
| **Cites** | specs/13.31 · arch/10-manager/03 §The footer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | metadata whose `id` is 49 bytes, whose `version` is 33, whose `arch` is 13 and whose `core` is 25 |
| **Action** | produce a package from each |
| **Expected** | each is refused, naming the field and its width; nothing is written |

## yoke:the-package.06 — an algorithm this reader does not know cannot be decided, and says which

| Field | Value |
| --- | --- |
| **Cites** | specs/13.11 · arch/10-manager/03 §The algorithm identifiers |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | footers naming `hash_id` 9, `sig_id` 9 and `seal_id` 9 |
| **Action** | check each |
| **Expected** | each is reported as undecidable, naming the field and the value — distinct from a check that decided against the file |

## yoke:the-package.07 — a signature over the metadata is verified against a trusted key, Ed25519 and RSA alike

| Field | Value |
| --- | --- |
| **Cites** | specs/13.9 · specs/13.10 · arch/10-manager/03 §Three checks, three different things · arch/10-manager/03 §The algorithm identifiers |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package signed with Ed25519 and one signed with RSA-2048, each key trusted |
| **Action** | check each |
| **Expected** | each passes; `key_id` is the SHA-256 of the signing public key |

## yoke:the-package.08 — altered metadata fails the signature even with its CRC recomputed

| Field | Value |
| --- | --- |
| **Cites** | specs/13.7 · specs/13.10 · arch/10-manager/03 §Three checks, three different things |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a signed package whose metadata had its `digest` replaced and its CRC recomputed |
| **Action** | check it |
| **Expected** | the CRC passes and the signature fails: it is a failure of trust |

## yoke:the-package.09 — a key the host does not trust is a failure of trust, decided on the footer alone

| Field | Value |
| --- | --- |
| **Cites** | specs/13.12 · specs/13.21 · arch/10-manager/03 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package signed by a key absent from the trusted set, read from a reader that fails any read before the last 512 bytes |
| **Action** | check it |
| **Expected** | it is a failure of trust naming the key; no byte of the payload was read |

## yoke:the-package.10 — an unsigned package is refused where the policy requires a signature, and checked where it does not

| Field | Value |
| --- | --- |
| **Cites** | specs/13.9 · specs/13.30 · arch/10-manager/03 §The order · arch/10-manager/03 §The policy about signatures |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an unsigned package |
| **Action** | check it under a policy that requires a signature, and under one that does not |
| **Expected** | the first is a failure of trust; the second passes on to the comparisons against the host |

## yoke:the-package.11 — architecture, model and Core range are compared against the host, each by name

| Field | Value |
| --- | --- |
| **Cites** | specs/13.3 · specs/13.24 · specs/13.25 · arch/10-manager/03 §The order · arch/10-manager/03 §Architecture, identity and versions |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a host on `aarch64` reading model 1, executing Core 2.4.0; packages for `x86_64`, for model 2, for `>=3.0`, and one that matches |
| **Action** | check each |
| **Expected** | the first three are refused as unwanted by this host, naming the architecture, the model and the Core range in turn; the last passes; no value means any architecture |

## yoke:the-package.12 — a version that is not a semantic version is refused; a pre-release is not

| Field | Value |
| --- | --- |
| **Cites** | specs/13.28 · specs/13.29 · arch/10-manager/03 §Architecture, identity and versions |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | packages whose version is `2.1`, `2.1.0-rc.1` and `2.1.0` |
| **Action** | check each |
| **Expected** | the first is refused, naming the version; the other two pass |

## yoke:the-package.13 — a sealed package opens with its credential, and the footer stays readable without it

| Field | Value |
| --- | --- |
| **Cites** | specs/13.14 · specs/13.15 · specs/13.16 · arch/10-manager/03 §Sealing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a package sealed with AES-256-GCM, signed |
| **Action** | check its footer holding no credential, then open it with the credential, with another, and with none; then open it with a tag altered by a byte |
| **Expected** | the footer is checked without the credential; it opens with its own and yields the payload; the other credential, no credential and the altered tag are each a refusal at unsealing |

## yoke:the-package.14 — the digest covers the payload as it is after unsealing

| Field | Value |
| --- | --- |
| **Cites** | specs/13.13 · specs/13.8 · arch/10-manager/03 §The order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | one payload, produced once in clear and once sealed |
| **Action** | read both footers' `digest`, then open a package in clear whose payload had a byte changed |
| **Expected** | the two digests are equal; the altered payload is refused as not the payload the metadata describes |

## yoke:the-package.15 — nothing in the verification reads the clock

| Field | Value |
| --- | --- |
| **Cites** | specs/13.12 · arch/10-manager/03 §The footer |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two packages identical but for `built_at`, one dated in 1970 and one a century ahead |
| **Action** | check and open each |
| **Expected** | both pass alike; `built_at` is read back as written |

## yoke:the-package.16 — a package is named by its identity and version

| Field | Value |
| --- | --- |
| **Cites** | specs/13.27 · arch/10-manager/03 §The figures this document sets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | metadata for `com.yoke.station` at `2.1.0` |
| **Action** | ask for the package's file name |
| **Expected** | it is `com.yoke.station-2.1.0.yoke` |
