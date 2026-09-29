# Reaching the administrative surface, and the actor

| | |
| --- | --- |
| **Feature** | the two administrative sockets, `operator.sock` and `shell.sock`, bound at the channels step with the form's mode; the kernel's peer credential resolved to the actor through the host's account database, a number where no name resolves and never a refusal; established once per connection on the shell projection and once per call on the operator projection; no request carrying an actor; and a connection as an object — an identity never reused, a projection, an actor and an opening instant, announced when it opens and when it closes, and closed by the Core when its transport stops answering |
| **Planning item** | yoke-project/yoke#92 |

## yoke:reaching-the-surface.01 — both sockets are bound at the channels step, with the form's mode

| Field | Value |
| --- | --- |
| **Cites** | specs/60.7 · specs/60.60 · specs/60.11 · arch/60-administrative-surface/01 §One pair per instance · arch/60-administrative-surface/03 §Reaching the socket is the whole of it |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an instance in the service form, and another in the application form |
| **Action** | run each through the trunk up to readiness |
| **Expected** | each has `operator.sock` and `shell.sock` under its root, both sockets, of mode `0660` in the service form and `0600` in the application form |

## yoke:reaching-the-surface.02 — the actor is the account the kernel names, and a number where no name resolves

| Field | Value |
| --- | --- |
| **Cites** | specs/60.14 · specs/60.15 · specs/60.16 · arch/60-administrative-surface/03 §Establishing which person acted |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the shell projection served on a socket; then served with an account database that resolves nothing |
| **Action** | connect to each |
| **Expected** | the first opening names an operator whose person is the name of the account the test runs under; the second an operator whose person is `uid:` followed by that account's number — the connection is opened either way |

## yoke:reaching-the-surface.03 — once per connection on the shell projection, once per call on the operator projection

| Field | Value |
| --- | --- |
| **Cites** | specs/60.15 · specs/60.19 · arch/60-administrative-surface/03 §Establishing which person acted |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | both projections served, with an account database that counts what it is asked |
| **Action** | make two calls over one connection to the operator projection; open one shell connection and send two requests on it |
| **Expected** | the operator's two calls asked the database twice; the shell connection asked it once |

## yoke:reaching-the-surface.04 — no request carries an actor, at any level of the contract

| Field | Value |
| --- | --- |
| **Cites** | specs/60.14 · arch/60-administrative-surface/03 §Establishing which person acted · arch/45-events/01 §The actor is established, never claimed |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the definitions of a request and of a client's frame |
| **Action** | walk every message they reach, field by field |
| **Expected** | no field is named `actor` and none is of the actor's type |

## yoke:reaching-the-surface.05 — a connection has an identity never reused, and is announced when it opens and when it closes

| Field | Value |
| --- | --- |
| **Cites** | specs/60.40 · specs/60.39 · arch/60-administrative-surface/03 §A connection as an object · arch/60-administrative-surface/03 §What makes a connection a subject · arch/45-events/06 §The set |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the shell projection served, publishing what it concludes |
| **Action** | open two connections; read the surface's connections; close the first |
| **Expected** | the two have different identities; the surface lists both, each with the projection `shell`, the actor and when it opened; a `connection.opened` was published for each, about the connection, with the operator as its actor and `shell` as its projection; closing the first publishes `connection.closed` with the reason `cancelled`, and the surface lists only the second |

## yoke:reaching-the-surface.06 — a transport that stops answering is closed by the Core, on the project's liveness

| Field | Value |
| --- | --- |
| **Cites** | specs/60.41 · arch/60-administrative-surface/03 §A connection as an object |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the surface's servers |
| **Action** | read the liveness they are built with |
| **Expected** | a probe every 10 s, and a connection closed after three go unanswered: 30 s |

## yoke:reaching-the-surface.07 — through the Core, a shell connection opens as the account that made it, and is recorded

| Field | Value |
| --- | --- |
| **Cites** | specs/60.7 · specs/60.15 · arch/60-administrative-surface/01 §One pair per instance · arch/60-administrative-surface/03 §Establishing which person acted |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built and started in the service form |
| **Action** | connect to its `shell.sock`, then leave |
| **Expected** | both sockets are there at `0660`; the opening names the account the test runs under; the Core's output records `connection.opened` and then `connection.closed` about that connection, with the operator as the actor |
