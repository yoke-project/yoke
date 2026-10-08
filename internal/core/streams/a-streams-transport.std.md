# A stream's transport, and the two acts on it

| | |
| --- | --- |
| **Feature** | a stream flows because it was told to: the Core creates the stream's transport, chosen by the two declared tolerances, listens on it, and only then activates the stream on the Session; the ordered path carries one data envelope per packet, the framed path a 16-byte header and the payload per datagram; the transport is removed, and `unit.stream.stopped` published, on each of the three routes that end a stream; and `unit.stream.start` and `unit.stream.stop` are served on the administrative surface, with `unit.stream.activated` |
| **Planning item** | yoke-project/yoke#137 |

## yoke:a-streams-transport.01 — the transport is chosen by the two tolerances, created and listened on by the Core

| Field | Value |
| --- | --- |
| **Cites** | specs/50.86 · specs/50.88 · specs/50.92 · arch/50-plugin-surface/07 §A stream flows because it was told to · arch/50-plugin-surface/07 §What the two tolerances select · arch/30-core/04 |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance root, and three streams of one unit: one tolerating nothing, one tolerating loss, one tolerating reorder only |
| **Action** | open each stream's transport |
| **Expected** | each is a socket at `plugins/<unit>/streams/<stream>.sock` the Core listens on: an ordered packet socket for the stream that tolerates nothing, a framed datagram socket for the other two; opening an open stream again is refused; the streams open are listed for the unit |

## yoke:a-streams-transport.02 — the ordered path carries one data envelope per packet, in order, from one connection

| Field | Value |
| --- | --- |
| **Cites** | specs/50.90 · specs/50.91 · arch/50-plugin-surface/07 §No stream travels on the Session · arch/50-plugin-surface/08 §The other transports |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open ordered transport |
| **Action** | connect, and send three envelopes each carrying a data message, sequences 1 to 3; connect a second time; send a packet that is not a data envelope |
| **Expected** | the three are read in order, each with its sequence and payload unchanged; the second connection is closed at once and the first is untouched; the packet that is not a data envelope is not delivered and is recorded on the Core's side |

## yoke:a-streams-transport.03 — the framed path reads a 16-byte header and the payload, and a gap is a fault only where loss is not tolerated

| Field | Value |
| --- | --- |
| **Cites** | specs/50.87 · arch/50-plugin-surface/07 §The frame · arch/00-system/05 §The encoding, and the framing |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two open framed transports, one tolerating loss and one tolerating reorder only |
| **Action** | on each, send frames with sequences 1, 2 and 4, each a little-endian header of sequence and clock followed by a payload; and a datagram shorter than the header |
| **Expected** | every whole frame is read with its sequence, clock and payload unchanged; the gap is recorded as a fault on the stream that does not tolerate loss, and not on the other; the short datagram is not delivered and is recorded |

## yoke:a-streams-transport.04 — each of the three routes that end a stream removes its transport, and says why

| Field | Value |
| --- | --- |
| **Cites** | specs/50.97 · arch/50-plugin-surface/07 §Three routes end a stream, and the transport goes in all three · arch/45-events/06 §The set |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | open transports of one unit, and a transport of another |
| **Action** | close one stream as asked; then close every stream of the unit because its Session ended; then the other unit's because it exited |
| **Expected** | each socket is gone and a connection to it is refused; one `unit.stream.stopped` is published per stream closed, naming the stream and the reason — `asked` at severity 10, `session ended` and `unit exited` at 30; closing a stream that is not open publishes nothing |

## yoke:a-streams-transport.05 — the Session carries the activation and the instruction that stops a stream, and hands back the acknowledgement

| Field | Value |
| --- | --- |
| **Cites** | specs/50.93 · arch/50-plugin-surface/07 §A stream flows because it was told to · arch/50-plugin-surface/05 §The eight |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an open Session whose unit is granted one stream and not another |
| **Action** | activate the granted stream with a transport and an address; the unit accepts it; stop it, and the unit says it is done; activate the stream not granted |
| **Expected** | the unit receives an activation carrying the stream, the transport and the address, and then a stop naming the stream; each caller is handed the unit's acknowledgement; the stream not granted is refused with `scope.withheld` and nothing reaches the unit |

## yoke:a-streams-transport.06 — unit.stream.start creates the transport, then activates the stream, and answers

| Field | Value |
| --- | --- |
| **Cites** | specs/60.27 · specs/60.34 · arch/60-administrative-surface/04 §Every operation terminates in the Core · arch/60-administrative-surface/04 §What every answer carries · arch/45-events/06 §The set · arch/60-administrative-surface/07 §The shape is the corpus's |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running Plugin unit with a Session, whose Manifest declares two streams of which its plugin was granted one |
| **Action** | `unit.stream.start` for a unit nobody declared, for a stream not declared, for the stream not granted, and for the granted one, twice; then for a unit with no Session |
| **Expected** | `subject.unknown`, `stream.undeclared` naming the stream, `scope.withheld` naming it with a message that does not repeat its code, and `unit.no_session`, each leaving no transport; the granted stream answers a change effective immediately, previously `stopped`, with one consequence for the unit, after the transport existed and before the answer the unit was handed the activation, `unit.stream.activated` is published naming the stream, and the unit's record lists it; the second start answers that it was previously `activated` and changes nothing |

## yoke:a-streams-transport.07 — unit.stream.stop instructs the unit, then removes the transport

| Field | Value |
| --- | --- |
| **Cites** | specs/60.27 · specs/60.34 · arch/60-administrative-surface/04 §Every operation terminates in the Core · arch/50-plugin-surface/07 §Three routes end a stream, and the transport goes in all three |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a running Plugin unit with one stream flowing |
| **Action** | `unit.stream.stop` for it, twice |
| **Expected** | the unit is handed the instruction that stops the stream; the answer is a change previously `activated`, and the transport is gone; `unit.stream.stopped` is published with the reason `asked`; the unit's record no longer lists the stream; the second stop answers that it was previously `stopped` and changes nothing |

## yoke:a-streams-transport.08 — through the Core, a stream started flows from the unit and stops

| Field | Value |
| --- | --- |
| **Cites** | specs/50.86 · specs/50.90 · arch/50-plugin-surface/07 §A stream flows because it was told to |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition running one unit of a program that registers, opens its Session, reports its health, accepts an activation by connecting to the address it names and sending three data messages, and accepts a stop |
| **Action** | on the operator projection, grant the plugin its stream's capability and restart the unit; then `unit.stream.start` for its stream, and `unit.stream.stop` |
| **Expected** | the start is answered as a change and `unit.stream.activated` is published; the stop is answered, the stream's socket is gone, and the Core's output records the stream stopped as asked, with three data messages read |
