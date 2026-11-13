# The installation report

| | |
| --- | --- |
| **Feature** | an issue form, *I installed Yoke*, by which somebody who installed Yoke says so: which form, which host or board, which release, and nothing else; the project counts them |
| **Planning item** | yoke-project/yoke#432 |

## yoke:the-installation-report.01 — the form asks which form, which host or board, which release, and nothing else

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §Where the numbers start |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | read `.github/ISSUE_TEMPLATE/i-installed-yoke.yml` |
| **Expected** | it is named *I installed Yoke*, its issues are titled so and labelled `installed`; it holds exactly three questions, each required — `form`, a choice between the application form and the service form; `where`, the host or board; `release`, the release installed — and beside them at most text that asks nothing |

## yoke:the-installation-report.02 — the form is offered beside a blank issue

| Field | Value |
| --- | --- |
| **Cites** | prj_structure/85 §Where the numbers start |
| **Level** | L1 |
| **Method** | check |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a clean checkout |
| **Action** | read `.github/ISSUE_TEMPLATE/config.yml`, where there is one |
| **Expected** | there is none, or it does not turn blank issues off: the form is one more way in, and a report that fits no form is still welcome |
