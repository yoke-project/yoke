# A need's channel

| | |
| --- | --- |
| **Feature** | the channel the Core binds for a unit's need of class `unit`: a local socket of its own, in a directory of its own, on which the using unit is the one client of the unit its need is bound to — seeing that unit alone, asking only the questions and following only the streams its need uses, never commanding, and served nothing a declared channel's client alone may ask |
| **Planning item** | yoke-project/yoke#485 |

## yoke:a-need-s-channel.01 — a need's channel answers for the unit bound and for what the need uses, and for nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/70.52 · specs/70.55 · specs/70.56 · specs/70.57 · arch/70-interface-surface/10 §What it is · arch/70-interface-surface/10 §What it sees, and what it may address |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a need's channel of the unit `archive`, its need `source` bound to `acquire` and using the question `head-status` and the stream `station.spectra`, over a bench of running Plugin units |
| **Action** | ask `head-status` of `acquire`; ask it of `idle`; command `calibrate` of `acquire`; start `station.preview` of `acquire`; read the units; authenticate |
| **Expected** | the socket is `0600` in a directory `0700`; the question is answered; then `subject.unknown` naming `idle`, `use.command`, `use.undeclared` naming `station.preview`; the read holds `acquire` alone; `authenticate` is `operation.unknown` |
