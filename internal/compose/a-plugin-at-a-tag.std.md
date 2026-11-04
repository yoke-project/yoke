# Taking a Plugin from a repository at a tag

| | |
| --- | --- |
| **Feature** | `yoke-compose add plugin <url>@<tag>`: a tag that is not SemVer refused before anything is obtained, the repository's tag resolved to its commit and the source obtained at that commit, both the tag and the commit recorded; `update plugin <id> --to <tag>` taking the same source at another tag, replacing the two files, the digest and the record; `plugins` listing what was taken and whether each tag still names the commit recorded; no index is read |
| **Planning item** | yoke-project/yoke#402 |

## yoke:a-plugin-at-a-tag.01 — a repository at a tag is taken at the tag's commit, and both are recorded

| Field | Value |
| --- | --- |
| **Cites** | specs/95.34 · specs/95.40 · specs/95.41 · arch/20-composer/07 §Taking a Plugin into an application · arch/20-composer/07 §`plugins.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an application project made by `init`; a git repository reached by a `file://` URL, holding a Plugin project at its root with tags `v1.0.0` and `v1.1.0` on different commits, its `main` ahead of both |
| **Action** | `add plugin <url>@v1.0.0 --yes` |
| **Expected** | exit 0; the executable placed is the one the source at `v1.0.0`'s commit builds, not `main`'s; the entry in `plugins.yaml` holds the URL as given, `tag: v1.0.0` and that commit in full; nothing but the repository named was read |

## yoke:a-plugin-at-a-tag.02 — a tag that is not a version, or that the repository does not have, is refused

| Field | Value |
| --- | --- |
| **Cites** | specs/95.40 · arch/20-composer/07 §The verbs |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the projects of `.01`; the repository also tagged `latest` and `1.0` |
| **Action** | `add plugin --yes` `<url>@latest`, `<url>@1.0`, `<url>@v9.0.0`, `<url>` with no tag, and a URL that answers nothing |
| **Expected** | each exits non-zero, naming respectively the two tags as not SemVer — before anything is obtained —, the tag the repository does not have, the missing tag, and the source that cannot be obtained; the project is byte for byte as it was |

## yoke:a-plugin-at-a-tag.03 — `update plugin` takes another tag and replaces what the first one placed

| Field | Value |
| --- | --- |
| **Cites** | specs/95.42 · arch/20-composer/07 §The verbs · arch/20-composer/07 §`plugins.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01` after `v1.0.0` was taken; the composer's clock fixed |
| **Action** | build the package; `update plugin com.example.greeter --to v1.1.0 --yes`; build it again; then `update` to `latest` |
| **Expected** | the first package carries `v1.0.0`'s executable, and nothing changed until `update` was run; after it, the executable, the Manifest and the pinned digest are `v1.1.0`'s, the unit keeps its name and its other declarations, and the entry holds `v1.1.0`, its commit, the new digest and the fixed time; the second package carries the new executable; `update` to `latest` is refused as not SemVer and leaves the project as it was |

## yoke:a-plugin-at-a-tag.04 — `plugins` lists what was taken, and says when a tag has moved

| Field | Value |
| --- | --- |
| **Cites** | specs/95.41 · arch/20-composer/07 §The verbs · arch/20-composer/07 §`plugins.yaml` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the project of `.01` after `v1.0.0` was taken, and a second Plugin taken from a path |
| **Action** | `plugins`; move the repository's tag `v1.0.0` to another commit; `plugins` again; `update plugin com.example.greeter --to v1.0.0 --yes` |
| **Expected** | the first listing gives, for each, the identity, the unit, the source, the tag (none for the path) and the commit, and says the tag still names it; the second says `v1.0.0` now names another commit, both commits stated, and changes nothing in the project; the `update` reports the same before it builds, and records the commit the tag names now |
