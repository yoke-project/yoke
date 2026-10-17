# The conformance report a release carries

| | |
| --- | --- |
| **Feature** | the report a programs release carries: which family publication each run measures — the newest the manifest names for each family, at the commit its line names — the reports of those runs joined into one, `yoke-conformance-report-<version>.json`, and that file handed over beside the release's other files and named by its own line |
| **Planning item** | yoke-project/yoke#339 |

## yoke:the-conformance-report.01 — what is measured is each family's newest publication, at the commit its line names

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The report a release carries |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a manifest holding, out of order, three publications of the Go family, two of the Rust family's crate, one of the C family's source archive, the definitions' crate, and the programs' own lines; no publication of the Python or C++ family |
| **Action** | read what is to be measured |
| **Expected** | three entries, one per family that published — the Go family at its newest version, the Rust family's crate at its newest, the C family's source archive — each naming the family's repository, the version and the commit of that line; nothing for the definitions or the programs, and nothing for a family with no publication |

## yoke:the-conformance-report.02 — the joined report holds each family's reports, and a family that wrote none is named with why

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The report a release carries · arch/90-sdks/03 §What a run produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a directory per measured family: one holding two reports the suite wrote; one holding no report and the reason its run stopped; one holding no report and no reason |
| **Action** | join them for `v0.8.0` |
| **Expected** | one document for `v0.8.0` with an entry per family, in the order measured, each naming its repository, version and commit: the first holding its two reports as the suite wrote them, the second the reason, the third that its conformance script wrote no report; and it says not everything passed |

## yoke:the-conformance-report.03 — a programs tag with reports to join hands the report over beside its files, named by its own line

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §The report a release carries · arch/90-sdks/03 §What a run produces |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a repository tagged `v0.1.0`, and the directory a release's measuring leaves: what it set out to measure, one family, and that family's reports |
| **Action** | run the verb with the directory |
| **Expected** | the upload holds `yoke-conformance-report-0.1.0.json` beside the archives, holding that family's reports; one line names it, as `yoke-conformance-report`, with its digest and where it was handed over |
