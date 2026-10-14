# The session token

| | |
| --- | --- |
| **Feature** | the token a routable channel issues once the host has accepted a caller: 32 random bytes, held in the Core's memory against one account and one channel, for `12h`; carried as a cookie the page cannot read in the browser projection and as an opaque string in the typed one; ended early by a fresh authentication of the same account, by the channel being unbound and by the Core stopping |
| **Planning item** | yoke-project/yoke#278 |

## yoke:the-session-token.01 — an accepted password is answered with a token: an opaque string in the typed projection, a cookie the page cannot read in the browser's

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The session token · arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a routable `local` channel and a routable `http+ws` channel, whose host accepts `alice` with `hunter2x`, the Core's clock fixed |
| **Action** | attach to the first sending `authenticate` with the password; `POST /v1/authenticate` with it to the second |
| **Expected** | the first's answer names `alice`, a token of 32 bytes as base64url, and an expiry twelve hours after the clock; the second's sets the cookie `yoke-session` — `HttpOnly`, `SameSite=Strict`, `Secure`, path `/v1` — and its body names `alice` and the expiry and carries no token |

## yoke:the-session-token.02 — a token attaches, in either projection, as the account it stands for

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The session token · arch/70-interface-surface/03 §Who the client is |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the two channels of `.01`, and a token each issued for `alice` |
| **Action** | attach to the first sending `authenticate` with the token; open `/v1/events` on the second with the cookie; then open `/v1/events` with no cookie, and with a cookie holding a token never issued |
| **Expected** | the first two are attached, and `channel.attached` names `alice` for each; the third is refused `auth.required` and the fourth `auth.invalid` |

## yoke:the-session-token.03 — a token is bound to the channel that issued it

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The session token |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | two routable `local` channels, `panel` and `bench`, and a token issued on `panel` |
| **Action** | attach to `bench` with that token |
| **Expected** | refused `auth.invalid` |

## yoke:the-session-token.04 — a token ends at twelve hours, and earlier when its account authenticates again or the channel is unbound

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/04 §The session token · arch/80-security/04 §The figures this document sets |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a routable `local` channel, the Core's clock under the case's hand; a token issued for `alice` |
| **Action** | move the clock eleven hours and attach with it; move it past twelve and attach again; issue a second token for `alice`, then authenticate `alice` once more and attach with the second; unbind the channel, bind it again, and attach with the token the last authentication issued |
| **Expected** | the first attaches; the second, third and fourth are refused `auth.invalid` |

## yoke:the-session-token.05 — through the Core, a browser signs in and attaches with its cookie

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/08 §2 — A browser reaches a routable channel |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a real Core, `yoke-auth` beside it asking a stack of the test's own that accepts `alice` with `hunter2x`; its composition declares a routable `http+ws` channel on `127.0.0.1`, its material in the store |
| **Action** | over HTTPS, `POST /v1/authenticate` with the password, keeping cookies; then open `/v1/events` as a WebSocket with the cookie |
| **Expected** | the first sets `yoke-session`; the WebSocket opens and its first frame is the opening picture |
