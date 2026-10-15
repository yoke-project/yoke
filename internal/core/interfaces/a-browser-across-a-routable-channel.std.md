# A browser across a routable channel

| | |
| --- | --- |
| **Feature** | the second of the security flows end to end with the built programs: a browser client, holding cookies and trusting only the certificate its provisioner gave it, reaches a routable `http+ws` channel over TLS 1.3 negotiating `http/1.1`, signs in with a password the host's authentication service accepts, attaches over WSS with the cookie the page cannot read, and sends a command that the unit's granted scope decides — refused while the capability is withheld, carried to the unit once it is granted |
| **Planning item** | yoke-project/yoke#280 |

## yoke:a-browser-across-a-routable-channel.01 — a browser signs in, attaches over WSS, and its command is decided by the unit's granted scope

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/08 §2 — A browser reaches a routable channel · arch/80-security/04 §The session token · arch/70-interface-surface/08 §`http+ws` — the browser projection |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a real Core, `yoke-auth` beside it asking a stack of the test's own that accepts `alice` with `hunter2x`; its composition declares a Plugin unit `acquire` declaring the command `calibrate`, whose capability is not granted, and a routable `http+ws` channel `service` on `127.0.0.1`, its certificate and key in the store; a client holding a cookie jar, whose TLS configuration trusts that certificate alone and offers `http/1.1` |
| **Action** | `GET /v1/events` with no cookie; `POST /v1/authenticate` with the password; open `/v1/events` as a WebSocket with the jar; `POST /v1/command` of `calibrate` to `acquire`; grant `command.calibrate.accept` to the unit's Plugin and restart the unit through the operator projection; `POST /v1/command` again |
| **Expected** | the connection is TLS 1.3 with `http/1.1` negotiated and the server verified against the given certificate; the first request is refused `auth.required`; the sign-in sets `yoke-session` — `HttpOnly`, `SameSite=Strict`, `Secure` — and its body carries no token; the WebSocket's first frame is the opening picture; the first command is refused `scope.withheld` with `403`; the second is acknowledged done with the unit's line |
