# A Go client across a routable address

| | |
| --- | --- |
| **Feature** | the published Go interface library against a real Core on a routable channel of the typed projection: TLS 1.3 against the roots the client was given, the credential first, the session token its answer carries presented to a later attachment, and a secret the host refuses refused |
| **Planning item** | yoke-project/yoke-sdk-go#60 |

## yoke:a-go-client-across-a-routable-address.01 — through the Core, the Go library signs in, and attaches again with the token it was given

| Field | Value |
| --- | --- |
| **Cites** | arch/80-security/03 §What the two projections look like on a routable address · arch/70-interface-surface/03 §Who the client is · arch/80-security/04 §The session token |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a real Core, `yoke-auth` beside it asking a stack of the test's own that accepts `alice` with `hunter2x`; its composition declares a routable `local` channel on `127.0.0.1`, its material in the store; the library given the channel's certificate as its roots |
| **Action** | attach with the library presenting `alice` and `hunter2x`, close; attach presenting the token the first answer carried; attach presenting `alice` and another secret |
| **Expected** | the first two surface the opening and the account `alice`, the first with a token and an expiry; the third is the library's refusal carrying `auth.invalid` |
