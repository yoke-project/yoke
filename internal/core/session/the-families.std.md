# The eight families

| | |
| --- | --- |
| **Feature** | the closed set of families that travel in a Session, each only in the direction it may travel in: what the unit originates is received, what the Core originates is refused from the unit and is all the Core sends; an acknowledgement answers a command and an answer a question; an acknowledgement is accepted, done or failed, an acceptance followed by at most one final answer; and whoever asked is handed the first answer the unit sends |
| **Planning item** | yoke-project/yoke#83 |

## yoke:the-families.01 — every family a unit may originate is received

| Field | Value |
| --- | --- |
| **Cites** | specs/50.61 · specs/50.62 · specs/50.63 · arch/50-plugin-surface/05 §The eight |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session to which the Core has sent a command and a question |
| **Action** | the unit sends a health report, an event, an uncorrelated error, an acknowledgement of the command, an answer to the question, an error correlated to the command, and a session close |
| **Expected** | nothing is answered until the close, which ends the Session as closed by the unit |

## yoke:the-families.02 — a family the Core originates is refused from a unit, whatever it says

| Field | Value |
| --- | --- |
| **Cites** | specs/50.68 · specs/50.73 · arch/50-plugin-surface/05 §Direction is a rule of its own |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session |
| **Action** | the unit sends a command, a stream activation, a question, and a session revocation |
| **Expected** | each is answered `session.direction`, correlated to it; the Session stays open |

## yoke:the-families.03 — the Core originates its own families and none of the unit's

| Field | Value |
| --- | --- |
| **Cites** | specs/50.68 · arch/50-plugin-surface/05 §Direction is a rule of its own |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session |
| **Action** | the Core is asked to send a health report, an acknowledgement, data, an event, an answer, a session open, a session close and a session revocation; then a command, a question and an error |
| **Expected** | each of the first eight is refused where it was asked for and nothing reaches the unit; a revocation goes only by revoking; the command, the question and the error arrive, in that order |

## yoke:the-families.04 — an acknowledgement answers a command, and an answer a question

| Field | Value |
| --- | --- |
| **Cites** | specs/50.61 · specs/50.69 · specs/50.102 · arch/50-plugin-surface/08 §The eight payloads |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session to which the Core has sent a command and a question |
| **Action** | the unit acknowledges the question, answers the command, then acknowledges the command and answers the question |
| **Expected** | the first two are answered `session.correlation.unknown`, correlated to each; the last two are received with no answer |

## yoke:the-families.05 — an acceptance is followed by at most one final answer, and a final answer ends the exchange

| Field | Value |
| --- | --- |
| **Cites** | specs/50.102 · arch/50-plugin-surface/05 §What an acknowledgement says |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session to which the Core has sent four commands |
| **Action** | to the first the unit sends accepted, done, then done again; to the second accepted twice; to the third failed, then accepted; to the fourth an acknowledgement with no outcome |
| **Expected** | accepted and done on the first are received; its second done is answered `session.correlation.unknown`; the second acceptance of the second is answered `session.message.malformed`; the acceptance after failed is answered `session.correlation.unknown`; the acknowledgement with no outcome is answered `session.message.malformed` — each correlated to the offending message |

## yoke:the-families.06 — whoever asked is handed the first answer the unit sends

| Field | Value |
| --- | --- |
| **Cites** | specs/50.102 · arch/50-plugin-surface/05 §What an acknowledgement says |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session, and a unit that accepts one command and later reports it done, carries out a second at once, answers a question, and says nothing to a third command |
| **Action** | the Core issues each command and the question, each with a wait of its caller's |
| **Expected** | the first caller is handed the acceptance, and the later done reaches no caller and is recorded on the Core's side; the second is handed done; the question's caller is handed the answer; the third caller is told the wait ran out, and the command's exchange stays open for an acknowledgement that arrives late |

## yoke:the-families.07 — through the Core, a unit that sends a command is refused for being one

| Field | Value |
| --- | --- |
| **Cites** | specs/50.68 · arch/50-plugin-surface/05 §Direction is a rule of its own |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition running one unit of a program that registers, opens its Session, heartbeats, and sends a command |
| **Action** | start the Core |
| **Expected** | the unit receives `session.direction` correlated to its command, and the Core's output records the refusal |
