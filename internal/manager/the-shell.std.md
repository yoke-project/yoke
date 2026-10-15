# The shell

| | |
| --- | --- |
| **Feature** | `yoke shell`: the manager as a terminal on an instance's shell projection — one connection held until the person leaves; a line is a client verb as written after `yoke`, parsed and rendered as the verb does, with no vocabulary of its own; `--instance` on a line, `--input -`, a verb that does not speak to the Core and a word that is no verb refused, and the shell going on; the standing subscription's events printed as `* ` lines between answers; a following verb interrupted, which cancels that call and leaves the connection; exit statuses `0` closed by the person, `1` refused on opening, `2` unreachable or ended by the Core; nothing persisted |
| **Planning item** | yoke-project/yoke#279 |

## yoke:the-shell.01 — a line is a client verb, and anything else is refused while the shell goes on

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/01 §`yoke shell` — the terminal · arch/60-administrative-surface/02 §One contract, two shapes of exchange |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a shell projection that opens as `ada` on the connection `s-1` and records every call it is sent |
| **Action** | open `yoke shell` and type, one per line: nothing; `unit list --instance other`; `app list`; `frobnicate`; `unit ask bench status --input -`; `unit show "two words"`; `exit` |
| **Expected** | the shell prints `ada` and `s-1`; the second line is refused naming `--instance`, the third naming `app list`, the fourth naming `frobnicate`, the fifth naming `--input -`, and none of them is sent; the sixth is sent as a read of the unit `two words`; the shell exits `0`, and writes nothing outside its output |

## yoke:the-shell.02 — what ends the shell decides its exit status

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/01 §`yoke shell` — the terminal · arch/10-manager/01 §What it reports |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a shell projection that opens; a second that refuses the connection `auth.required`; a third that ends the connection after opening it; an instance whose shell socket is not there |
| **Action** | open `yoke shell` on each, the first with an input that ends with no `exit` |
| **Expected** | the first exits `0`; the second `1`, printing the refusal; the third `2`, saying the connection ended; the fourth `2`, saying the instance is not running |

## yoke:the-shell.03 — through the Core, answers, events and an interrupted follow share one connection

| Field | Value |
| --- | --- |
| **Cites** | arch/10-manager/01 §`yoke shell` — the terminal · arch/60-administrative-surface/02 §Interleaving, correlation and completion |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core running a composition of one Plugin unit `bench`, reached through the `core.yaml` it was started with |
| **Action** | open `yoke shell` and type `unit list`; `unit restart bench`; `log follow --unit bench`, then interrupt it; `unit show bench`; `exit` |
| **Expected** | `unit list` prints the unit's record as `yoke unit list` does; the restart prints what it changed, and the events it caused are printed as lines beginning `* `, none inside an answer; the interrupt ends the follow and not the shell, and `unit show bench` is answered after it on the same connection; the shell exits `0` |
