# The Session

| | |
| --- | --- |
| **Feature** | the accepted context everything after admission happens in: one bidirectional stream opened by a first envelope carrying the identity admission issued, used once, kept valid by heartbeats within the Core's terms, ended by the unit's close or the Core's revocation and never by a lost stream alone; every message in it carrying four header fields and one payload, correlated where it answers, and validated in order |
| **Planning item** | yoke-project/yoke#16 |

## yoke:the-session.01 — the first envelope is a session OPEN carrying the identity admission issued

| Field | Value |
| --- | --- |
| **Cites** | specs/50.2 · specs/50.39 · specs/50.47 · specs/50.48 · arch/50-plugin-surface/04 §Opening and closing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit admitted with a Session identity, and what the Session tells the lifecycle machine collected |
| **Action** | open the stream and send, first, a session `OPEN` carrying that identity; then a heartbeat |
| **Expected** | the stream stays open, and the unit's machine is told the Session opened |

## yoke:the-session.02 — anything else first, or an identity that resolves to nothing, closes the stream

| Field | Value |
| --- | --- |
| **Cites** | specs/50.40 · specs/50.47 · arch/50-plugin-surface/04 §Opening and closing · arch/50-plugin-surface/04 §Identity |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit admitted with a Session identity |
| **Action** | open a stream whose first envelope is a heartbeat; one whose `OPEN` carries an identity never issued; one that opens correctly; then another whose `OPEN` carries the identity already used |
| **Expected** | the first, second and fourth are closed with nothing sent to them; the third stays open; the machine is told of one opening only |

## yoke:the-session.03 — an identity is used once, and a Session that ended cannot be reopened

| Field | Value |
| --- | --- |
| **Cites** | specs/50.40 · specs/50.41 · arch/50-plugin-surface/04 §Identity |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session |
| **Action** | the unit closes it; then opens a new stream with the same identity |
| **Expected** | the new stream is closed with nothing sent to it |

## yoke:the-session.04 — an envelope carries four header fields and exactly one payload

| Field | Value |
| --- | --- |
| **Cites** | specs/50.54 · specs/50.55 · specs/50.57 · specs/50.58 · specs/50.60 · arch/50-plugin-surface/04 §The envelope carries four fields |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session |
| **Action** | send an envelope with no payload; one with no message identity; then have the Core send the unit a command |
| **Expected** | each of the first two is answered with an error `session.message.malformed`, the first correlated to its message identity; the command arrives with a message identity, the Session's identity and the Core's clock, and no correlation |

## yoke:the-session.05 — a receiver validates in order and stops at the first failure

| Field | Value |
| --- | --- |
| **Cites** | specs/50.73 · specs/50.74 · specs/50.75 · specs/50.56 · specs/50.46 · arch/50-plugin-surface/04 §Validation, in order |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session |
| **Action** | send a heartbeat naming another Session's identity; a heartbeat; another reusing its message identity; a control message from the unit reusing it again; a control message with a fresh identity |
| **Expected** | the first is dropped with no answer and recorded on the Core's side; the second passes; the third is answered `session.message.duplicate`, and so is the fourth — uniqueness is checked before direction; the fifth is answered `session.direction` |

## yoke:the-session.06 — a message that answers is correlated, within its Session, and never to itself

| Field | Value |
| --- | --- |
| **Cites** | specs/50.69 · specs/50.70 · specs/50.71 · specs/50.72 · arch/50-plugin-surface/04 §Correlation |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session to which the Core has sent a command |
| **Action** | the unit sends an acknowledgement with no correlation; one correlated to an identity the Core never sent; one correlated to its own identity; one correlated to the command |
| **Expected** | `session.correlation.missing`, then `session.correlation.unknown` twice, each correlated to the offending message; the last is accepted with no answer |

## yoke:the-session.07 — heartbeats within the Core's terms keep it valid, and too many misses revoke it

| Field | Value |
| --- | --- |
| **Cites** | specs/50.43 · specs/50.53 · specs/50.51 · arch/50-plugin-surface/04 §Validity · arch/50-plugin-surface/04 §Revocation |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a unit admitted with heartbeat terms of 100 ms and a tolerance of 3 |
| **Action** | open the Session and heartbeat every 50 ms for 600 ms; then stop heartbeating |
| **Expected** | the Session is valid throughout the 600 ms; within 500 ms of the last heartbeat the unit receives `REVOKED` with the cause `liveness lost`, the stream closes, and the machine is told the Session ended, not withdrawn |

## yoke:the-session.08 — losing the stream is not closing it

| Field | Value |
| --- | --- |
| **Cites** | specs/50.50 · specs/50.45 · arch/50-plugin-surface/04 §Opening and closing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session with heartbeat terms of 200 ms and a tolerance of 2 |
| **Action** | the unit drops the connection without closing |
| **Expected** | nothing is concluded at once — 100 ms later the machine has been told nothing; once the heartbeat window has passed, it is told the Session ended, not withdrawn |

## yoke:the-session.09 — an orderly close is the unit's, and a revocation is the Core's

| Field | Value |
| --- | --- |
| **Cites** | specs/50.49 · specs/50.51 · specs/50.103 · specs/50.52 · arch/50-plugin-surface/04 §Revocation · arch/50-plugin-surface/04 §Opening and closing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two open Sessions |
| **Action** | the first unit sends `CLOSE`; the Core revokes the second because its plugin was disabled, with a line for a person |
| **Expected** | the first's machine is told the Session ended, not withdrawn, and the Core's record says it was closed; the second unit receives `REVOKED` with the cause `plugin disabled` and the line, its stream closes, its machine is told the Session was withdrawn, and the record says it was revoked |

## yoke:the-session.10 — a registered unit opens its Session on the plugin channel

| Field | Value |
| --- | --- |
| **Cites** | specs/50.2 · specs/50.47 · arch/50-plugin-surface/01 §The order of the first four acts |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition running one unit of a program that registers, opens its Session with the identity it was given, heartbeats, and closes after a second |
| **Action** | start the Core |
| **Expected** | the Core's output says the unit's Session opened, and then that it was closed by the unit |
