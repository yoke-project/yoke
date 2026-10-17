# The shared ring

| | |
| --- | --- |
| **Feature** | the one object that is not a socket: an anonymous memory file the Core creates, sizes and labels, laid out as a 64-byte header and 32 slots of up to 1 MiB; a producer publishes a frame into its slot and wakes every waiter on one word; a reader reads frame by frame, checking each slot's sequence before and after the copy, and takes an overwritten slot or a lap as a gap; the producer's descriptor writes and the reader's cannot |
| **Planning item** | yoke-project/yoke#139 |

## yoke:the-shared-ring.01 — the ring is created sized and labelled, its header written

| Field | Value |
| --- | --- |
| **Cites** | specs/25.34 · specs/50.94 · specs/50.97 · arch/50-plugin-surface/07 §The shared ring · arch/50-plugin-surface/07 §Objects that are not sockets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a label `yoke-bench-acquire-station.spectra` |
| **Action** | create a ring |
| **Expected** | the memory file is `64 + 32 × (32 + 1 MiB)` bytes and carries the label; its header reads `YOKERING`, version 1, 32 slots, a capacity of 1 MiB, nothing published and the wake word at zero |

## yoke:the-shared-ring.02 — a frame published is read with its sequence, its clock and its payload, in order

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a ring, a producer and a reader |
| **Action** | publish three frames of different lengths and read them |
| **Expected** | the reader receives frames 1, 2 and 3 with the clock and the payload each was published with, and no gap |

## yoke:the-shared-ring.03 — a reader more than the slot count behind moves to the oldest frame held, and says what it lost

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a ring into which 40 frames were published before the reader read any |
| **Action** | read |
| **Expected** | the first frame read is frame 9, the oldest still held, after a gap of eight; every later frame follows in order |

## yoke:the-shared-ring.04 — a slot overwritten while it is read is a gap

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a reader that copies frame 1 while the producer laps the ring and writes frame 33 into the same slot |
| **Action** | finish the read |
| **Expected** | the frame is not delivered: the slot's sequence was not 1 after the copy, and the read reports a gap |

## yoke:the-shared-ring.05 — a frame larger than a slot is refused to the producer

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a ring of 1 MiB slots |
| **Action** | publish a frame of 1 MiB and one byte |
| **Expected** | it is refused, naming the capacity; nothing is published and the count stays where it was |

## yoke:the-shared-ring.06 — a waiting reader is woken by a publication

| Field | Value |
| --- | --- |
| **Cites** | arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a reader waiting for the next frame, with nothing published |
| **Action** | publish a frame from another goroutine |
| **Expected** | the reader returns the frame well before its bounded interval would have passed |

## yoke:the-shared-ring.07 — the producer's descriptor writes, and a reader's cannot

| Field | Value |
| --- | --- |
| **Cites** | specs/50.98 · arch/50-plugin-surface/07 §The shared ring |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a ring, the descriptor handed to the producer and the one the Core keeps |
| **Action** | map each for writing |
| **Expected** | the producer's maps for writing; the reader's is refused |
