# Streams delivered to clients

| | |
| --- | --- |
| **Feature** | a stream's data delivered to a client that subscribed to it: the answer to `stream.subscribe` says where the data arrives — a per-subscriber socket in the instance's tree on a channel bound as a local socket, or the attachment's own connection on one bound on an address — under a subscriber identity the Core counts, never a name the client chooses; every path carries the stream's sequence, the sender's clock and the payload unchanged; subscribing to a stream not flowing is legal; and a delivery is released by `stream.unsubscribe`, by the attachment ending and by the unit's Session ending, and kept, carrying nothing, while the stream is stopped |
| **Planning item** | yoke-project/yoke#145 |

## yoke:streams-delivered.01 — on a local-socket channel, the answer names a socket under the subscriber's counted identity

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §The three delivery paths · arch/30-core/04 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel on a local socket, and a Plugin unit granted a stream that is not flowing |
| **Action** | `stream.subscribe` the stream twice |
| **Expected** | each answer names a delivery and a socket at `plugins/<unit>/subscribers/<stream>/<n>.sock`, `n` decimal and zero-padded to eight digits, the second greater than the first; each says the stream is not flowing; each socket exists |

## yoke:streams-delivered.02 — what is read from the stream reaches every subscriber unchanged, in order

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/07 §What every path preserves |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two deliveries of one stream, each socket connected to |
| **Action** | the stream flows, and its transport reads three messages |
| **Expected** | each socket reads three packets, each the sequence and the clock, little-endian, then the payload, all as they were read and in order |

## yoke:streams-delivered.03 — on a channel bound on an address, the delivery arrives on the attachment's connection

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/07 §The three delivery paths · arch/70-interface-surface/08 §`local` — the typed projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment to a channel carrying `local` on loopback |
| **Action** | `stream.subscribe` a stream; the stream flows and its transport reads two messages |
| **Expected** | the answer names the delivery and says it arrives on the connection; two stream-delivery frames arrive carrying the delivery, the sequence, the clock and the payload unchanged |

## yoke:streams-delivered.04 — a delivery is released by unsubscribing, detaching or the Session ending, and kept while the stream is stopped

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/07 §What ends a delivery |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment with three deliveries of one stream, and another attachment with one |
| **Action** | unsubscribe the first; stop the stream and start it again; end the unit's Session; detach the other attachment |
| **Expected** | the first's socket is gone at once and the unsubscribe is answered; across the stop the second's socket stays, and it carries again once the stream flows; the Session ending removes the second's and third's sockets; detaching removes the other attachment's |

## yoke:streams-delivered.05 — a stream a channel may not address is refused

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/04 §What decides whether a client may issue one |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment, and a Plugin unit whose Plugin declares two streams of which one is granted |
| **Action** | `stream.subscribe` a unit nobody declared, a stream never declared, and the stream not granted |
| **Expected** | `subject.unknown` naming the unit, `scope.undeclared` and `scope.withheld` naming the stream; no socket is created |

## yoke:streams-delivered.06 — through the Core, a client subscribes, starts the stream, and reads its data

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §Two acts, and they are not the same one |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring a local channel and a Plugin unit of a program that sends three data messages when its stream is activated |
| **Action** | on the operator projection, grant the stream and restart the unit; attach, `stream.subscribe` the stream, connect to the socket named, then `stream.start` it |
| **Expected** | the socket reads the three messages, in order, each with its sequence and payload |

## yoke:streams-delivered.07 — a delivery whose client falls behind is released, and the client is told

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/07 §What every path preserves · arch/70-interface-surface/07 §What ends a delivery |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a delivery of a flowing stream on a local-socket channel, whose client connects and never reads, holding at most four frames |
| **Action** | the stream's transport reads far more than the socket and the delivery can hold |
| **Expected** | the delivery is released: its socket closes, so the client reads what reached it and then the end, never a gap; and unsubscribing it afterwards is refused, since the attachment no longer holds it |
