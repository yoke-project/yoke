# A need's channel

| | |
| --- | --- |
| **Feature** | the channel the Core binds for a unit's need of class `unit`: a local socket of its own, in a directory of its own, on which the using unit is the one client of the unit its need is bound to — seeing that unit alone, asking only the questions and following only the streams its need uses, never commanding, and served nothing a declared channel's client alone may ask |
| **Planning item** | yoke-project/yoke#485 |

## yoke:a-need-s-channel.01 — a need's channel answers for the unit bound and for what the need uses, and for nothing else

| Field | Value |
| --- | --- |
| **Cites** | specs/70.52 · specs/70.55 · specs/70.56 · specs/70.57 · specs/50.108 · specs/50.112 · arch/70-interface-surface/10 §What it is · arch/70-interface-surface/10 §What it sees, and what it may address · arch/50-plugin-surface/10 §The three fields of a question |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a need's channel of the unit `archive`, its need `source` bound to `acquire` and using the question `head-status` and the stream `station.spectra`, the use of the question granted and of the stream not, over a bench of running Plugin units |
| **Action** | ask `head-status` of `acquire`; ask it of `idle`; command `calibrate` of `acquire`; start `station.spectra` of `acquire`; start `station.preview`; read the units; authenticate |
| **Expected** | the socket is `0600` in a directory `0700`; the question is answered, and `acquire` is asked it by `unit:archive`; then `subject.unknown` naming `idle`, `use.command`, `scope.withheld` naming `station.spectra`, `use.undeclared` naming `station.preview`; the read holds `acquire` alone; `authenticate` is `operation.unknown` |

## yoke:a-need-s-channel.02 — a question caused by another takes its chain and its deadline, and a use refused or chained is reported

| Field | Value |
| --- | --- |
| **Cites** | specs/50.109 · specs/50.110 · specs/70.59 · specs/70.60 · arch/70-interface-surface/10 §A question asked on it · arch/70-interface-surface/10 §What is kept · arch/45-events/06 §`unit` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the need's channel of `.01`, the use of `head-status` granted, and `archive` holding two outstanding questions: `c-1`, of chain `c0ffee`, with 10 s left, and `c-2` with 550 ms left; a declared channel `panel` over the same bench |
| **Action** | on the need's channel ask `head-status` caused by `c-1`, then by `c-9`, then by `c-2`, and command `calibrate`; on `panel` ask `head-status` with no cause, then caused by `c-1` |
| **Expected** | the first reaches `acquire` with chain `c0ffee` and a deadline 500 ms short of `c-1`'s, and `unit.question.chained` is published on `acquire` with the chain, `unit:archive` and the type and no payload; then `use.cause.unknown`, `deadline.exhausted` without `acquire` being asked, and `use.command` with `unit.use.refused` on `archive` naming the need, the command and the code; on `panel` the question carries a new chain of 32 hex digits and a deadline about 30 s ahead, and the caused one is refused `use.cause.unknown` |

## yoke:a-need-s-channel.03 — a stream a need's channel follows is held against another channel's stop

| Field | Value |
| --- | --- |
| **Cites** | specs/70.58 · arch/70-interface-surface/10 §A stream it holds |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the need's channel of `archive`, using and granted `station.spectra` of `acquire`, and a declared channel `panel`, sharing one count of holds |
| **Action** | on the need's channel subscribe to `station.spectra`; on `panel` stop it; on the need's channel unsubscribe; on `panel` stop it again |
| **Expected** | the first stop is refused `stream.held`, naming `archive`; the second is answered |

## yoke:a-need-s-channel.04 — a need's channel opens on the unit bound and observes it alone

| Field | Value |
| --- | --- |
| **Cites** | specs/70.56 · arch/70-interface-surface/10 §What it sees, and what it may address |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a need's channel of the unit `archive`, its need `source` bound to `acquire`, over a bench of several Plugin units and an instance |
| **Action** | attach; then the instance begins to stop, `idle` stops and `acquire` stops |
| **Expected** | the opening holds no instance and no unit but `acquire`; the first event the standing subscription carries is `acquire`'s |
