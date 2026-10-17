# The trust store

| | |
| --- | --- |
| **Feature** | the file a host keeps its trust policy in, read by the manager at install: whether a signature is required, the set of keys trusted — each named by the digest of its own public key — and where the credential that opens a sealed package is kept; required to be root's and written by nobody else; an unknown key refused; a variable that may tighten the policy and never loosen it |
| **Planning item** | yoke-project/yoke#222 |

## yoke:the-trust-store.01 — the file is read: the policy, the keys and the seal

| Field | Value |
| --- | --- |
| **Cites** | specs/80.23 · specs/13.18 · arch/80-security/05 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file owned by root, mode 0644, requiring a signature, naming an Ed25519 key and an RSA-2048 key with labels, and a seal credential's path |
| **Action** | read it |
| **Expected** | the policy requires a signature, both keys are trusted under their `id`, the labels are kept and compared with nothing, and the credential is at rest at the path named; a file that says nothing but `keys: []` requires no signature and supplies the credential per invocation |

## yoke:the-trust-store.02 — a file not root's, or writable by anybody else, is refused as a policy file

| Field | Value |
| --- | --- |
| **Cites** | specs/80.26 · specs/13.22 · arch/80-security/05 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the same file owned by another account, then owned by root with mode 0664, then 0646 |
| **Action** | read each |
| **Expected** | each is refused, naming the owner or the mode; none is read leniently |

## yoke:the-trust-store.03 — a key whose `id` is not the digest of its own public key is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/13.21 · arch/80-security/05 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file whose second key carries the first key's `id` |
| **Action** | read it |
| **Expected** | it is refused, naming the row |

## yoke:the-trust-store.04 — an unknown key is refused at any level

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/05 §The file · arch/15-gate/05 §The formats |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | files with an unknown key at the top, in a key's row, and under `seal` |
| **Action** | read each |
| **Expected** | each is refused, naming the key and where it stands |

## yoke:the-trust-store.05 — an algorithm is `ed25519` or `rsa-2048`, and the public key must be one

| Field | Value |
| --- | --- |
| **Cites** | specs/13.21 · arch/80-security/05 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | rows naming `dsa`, naming `ed25519` over an RSA key, and naming `rsa-2048` over a key of 4096 bits |
| **Action** | read each |
| **Expected** | each is refused, naming the row and what it found |

## yoke:the-trust-store.06 — the set is what is trusted: a package signed by a key removed is refused, by one kept is checked

| Field | Value |
| --- | --- |
| **Cites** | specs/13.21 · specs/13.23 · specs/80.24 · specs/80.27 · arch/80-security/05 §Rotation is an addition followed by a removal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two keys trusted, packages signed by each; then the first key removed from the file |
| **Action** | check both packages against the store before and after |
| **Expected** | before, both pass; after, the package signed by the removed key is a failure of trust and the other passes |

## yoke:the-trust-store.07 — a variable may tighten the policy and never loosen it

| Field | Value |
| --- | --- |
| **Cites** | specs/80.23 · specs/13.19 · arch/80-security/05 §The file |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file that does not require a signature and one that does; `YOKE_REQUIRE_SIGNATURE` set to `1`, to `0`, to `false`, and unset |
| **Action** | read each file under each value |
| **Expected** | `1` requires a signature of the first file; no value makes the second file stop requiring one |

## yoke:the-trust-store.08 — a credential at rest is read from its path, which must be root's and `0600`, and is declared weaker

| Field | Value |
| --- | --- |
| **Cites** | specs/80.30 · specs/13.17 · arch/80-security/05 §The credential that opens a sealed package · arch/80-security/07 §What qualifies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a credential of 32 bytes in base64 at the path the file names, mode 0600; then mode 0640; then a credential of 16 bytes |
| **Action** | ask the store for the credential, given none at the invocation |
| **Expected** | the first yields the 32 bytes and the store reports `seal.credential.at_rest` among its weaker arrangements; the mode 0640 is refused naming the mode; the short credential is refused naming its length |

## yoke:the-trust-store.09 — with no path named, the credential is the one the invocation supplies

| Field | Value |
| --- | --- |
| **Cites** | specs/80.30 · specs/13.17 · arch/80-security/05 §The credential that opens a sealed package |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a file naming no seal credential |
| **Action** | ask the store for the credential, given one at the invocation and then given none |
| **Expected** | the first yields what was supplied, decoded; the second yields nothing, so a sealed package will be refused at unsealing; no weaker arrangement is reported |
