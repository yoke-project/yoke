# The browser projection

| | |
| --- | --- |
| **Feature** | the interface surface's `http+ws` projection: the same messages in protocol buffers' canonical JSON; attaching is opening `/v1/events`, a WebSocket carrying the opening and then the live shape, and setting the attachment's cookie, which the page cannot read; the operation name is the path, `POST /v1/<operation>` carrying one request and answered once, a refusal's structured error in the body and the transport's status a coarse hint; a control request with no live attachment refused `channel.not_attached`; a stream's delivery one binary WebSocket at the path its answer names; and anything else `404` |
| **Planning item** | yoke-project/yoke#147 |

## yoke:the-browser-projection.01 — attaching is opening /v1/events, which sets the attachment's cookie and carries the opening

| Field | Value |
| --- | --- |
| **Cites** | specs/70.11 · arch/70-interface-surface/08 §`http+ws` — the browser projection · arch/70-interface-surface/02 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel carrying `http+ws` on loopback |
| **Action** | open a WebSocket at `/v1/events`; then publish an event about a unit |
| **Expected** | the upgrade sets a cookie marked `HttpOnly` and `SameSite=Strict`, scoped to `/v1`; the first text message is the opening in JSON, at version 1; the next is the event, on the standing subscription |

## yoke:the-browser-projection.02 — the operation is the path, answered once in JSON, and a refusal carries its code in the body

| Field | Value |
| --- | --- |
| **Cites** | specs/70.12 · arch/70-interface-surface/08 §`http+ws` — the browser projection · arch/00-system/05 §How a refusal travels |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment open on `/v1/events`, its cookie held |
| **Action** | `POST /v1/read` with a read of the units; `POST /v1/read` with a read of a unit nobody declared; `POST /v1/read` carrying a subscription; `POST /v1/command` on a unit whose type was never declared |
| **Expected** | the first is answered `200` with the response in JSON; the second `404` with the refusal `subject.unknown` in the body; the third `400` with `operation.malformed`, the path and the request naming different operations; the fourth `403` with `scope.undeclared` |

## yoke:the-browser-projection.03 — a control request with no live attachment is refused channel.not_attached

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel carrying `http+ws` |
| **Action** | `POST /v1/read` with no cookie; open `/v1/events`, close it, and `POST /v1/read` with the cookie it set |
| **Expected** | both are answered `409` with `channel.not_attached` in the body |

## yoke:the-browser-projection.04 — a stream's delivery is one binary WebSocket at the path its answer names

| Field | Value |
| --- | --- |
| **Cites** | specs/70.6 · arch/70-interface-surface/07 §The three delivery paths · arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an attachment open on `/v1/events`, and a Plugin unit granted a stream |
| **Action** | `POST /v1/stream.subscribe`; open a WebSocket at the path answered; the stream's transport reads two messages |
| **Expected** | the answer names a path `/v1/streams/<unit>/<stream>/<n>`; the WebSocket carries two binary messages, each the sequence and the clock, little-endian, then the payload |

## yoke:the-browser-projection.05 — anything else is 404

| Field | Value |
| --- | --- |
| **Cites** | arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a channel carrying `http+ws` |
| **Action** | `GET /`, `GET /index.html`, `GET /v1/elsewhere` and `POST /v2/read` |
| **Expected** | each is answered `404` |

## yoke:the-browser-projection.06 — through the Core, a browser's two shapes reach a channel on loopback

| Field | Value |
| --- | --- |
| **Cites** | specs/70.11 · arch/70-interface-surface/02 §`http+ws` — the browser projection |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built, with a composition declaring an attached channel carrying `http+ws` on loopback and a unit that runs to completion |
| **Action** | open `/v1/events`; `POST /v1/read` the units with the cookie it set |
| **Expected** | the WebSocket's first message is the opening; the read is answered `200` with the unit's record |
