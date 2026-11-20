# What a use carries

| | |
| --- | --- |
| **Feature** | through a real Core, a Plugin unit that answers a question by asking another unit through the channel of its need: the question reaches the unit used with who asked and the chain, and the answer comes back — and nothing either carried is written to the Core's output or under the state directory, where only that a question was chained is kept |
| **Planning item** | yoke-project/yoke#489 |

## yoke:nothing-a-use-carries.01 — through the Core, a unit asks another through its need, and the answer carrying a secret is written nowhere

| Field | Value |
| --- | --- |
| **Cites** | specs/70.52 · specs/70.59 · specs/50.107 · specs/50.108 · specs/50.110 · specs/14.57 · arch/70-interface-surface/10 §What is kept · arch/70-interface-surface/10 §A question asked on it |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and logging at `debug`; a composition of `source`, a Plugin answering `head-status` with a random secret, who asked and the chain, and `archive`, a Plugin needing `unit:source` bound to `source`, handed `${bind.source}`, which answers `relay` by asking `source` through its need with the question as the cause; every capability granted and both units restarted |
| **Action** | on the administrative surface ask `archive` `relay`; wait until `unit.question.chained` is found under the state directory |
| **Expected** | the answer is the secret, asked by `unit:archive`, with a chain of 32 hex digits; the secret is in neither the Core's output nor any file under the state directory |
