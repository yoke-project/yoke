# The application invocation

| | |
| --- | --- |
| **Feature** | `yoke-core` executed in the application form: the invocation is the whole of what it receives — the instance's name, the bundle, the engine, the account's retention ceiling and the resolved parameter values; the descriptor is read once at the start, through the gate, and closed; its units are launched from the bundle with their references resolved; the version stamp is written at readiness; and an unverified run says so about itself |
| **Planning item** | yoke-project/yoke#234 |

## yoke:the-application-invocation.01 — an invocation that does not hold is refused before anything is created

| Field | Value |
| --- | --- |
| **Cites** | specs/20.8 · arch/30-core/01 §The application form · arch/30-core/03 §The eleven steps |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built; `XDG_RUNTIME_DIR` and `XDG_STATE_HOME` in directories of the test's own |
| **Action** | execute it with `--instance` and `--composition` together; with `--instance` and no `--bundle`; with `--param shared.label` holding no `=`; with `--retention bytes=lots`; with `--retention size=1MB` |
| **Expected** | each exits non-zero naming what does not hold, and the instance's runtime directory does not exist |

## yoke:the-application-invocation.02 — a Core executed on a bundle runs what the descriptor declares, with its references resolved

| Field | Value |
| --- | --- |
| **Cites** | specs/12.45 · specs/20.8 · arch/30-core/01 §The application form · arch/30-core/03 §The eleven steps · arch/15-gate/05 §How a name is resolved · arch/15-gate/03 §`units` |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a bundle whose descriptor, version `2.1.0`, declares `shared.label`, `shared.site` and `probe.label`, and a unit `probe` of kind `oneshot` whose `exec` is `components/probe` with its digest, whose `args` are `${label}` and `${site}` and whose `env` sets `OUT` to `${out}` |
| **Action** | execute the Core with `--instance bench-a`, `--bundle` naming it, and `--param` for `shared.label=Shared`, `shared.site=Bench A`, `probe.label=Inner` and `shared.out=<file>`; wait for the unit to run; stop it |
| **Expected** | it becomes ready; the unit is launched from inside the bundle and writes `Inner` and `Bench A` — the inner scope wins and one reference is one argument — to the file; the instance's state directory holds `version` saying `2.1.0`; it exits zero |

## yoke:the-application-invocation.03 — a descriptor or a value the gate refuses keeps the Core from starting

| Field | Value |
| --- | --- |
| **Cites** | specs/20.5 · specs/20.44 · arch/30-core/03 §The fatal boundary is a rule and not a number · arch/15-gate/06 §The phases |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the bundle of `.02`; a second bundle whose descriptor holds a key nobody declared |
| **Action** | execute the Core on the second bundle; then on the first with `--param probe.colour=red`, which no group declares |
| **Expected** | both exit non-zero naming the gate's finding — `key.unknown` — and neither binds a channel: no `operator.sock` exists under the instance's root |

## yoke:the-application-invocation.04 — the descriptor is read once and closed for the life of the incarnation

| Field | Value |
| --- | --- |
| **Cites** | specs/20.44 · specs/25.61 · arch/30-core/03 §When the description changes underneath |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core ready on the bundle of `.02` |
| **Action** | list the files the process holds open; replace the descriptor with one the gate would refuse; stop it |
| **Expected** | no descriptor of the process names the descriptor; the replaced file changes nothing — the process is still serving — and it exits zero |

## yoke:the-application-invocation.05 — an unverified run says so about itself

| Field | Value |
| --- | --- |
| **Cites** | specs/25.64 · arch/30-core/01 §The application form · arch/15-gate/06 §Phase 5 — weaker arrangements · arch/80-security/07 §What qualifies |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the bundle of `.02` |
| **Action** | execute the Core with `--unverified`; then without it |
| **Expected** | the first says, as a weaker finding, `bundle.unverified`, before it is ready; the second says nothing of it |

## yoke:the-application-invocation.06 — the engine the invocation names is the one the Core clears and launches on

| Field | Value |
| --- | --- |
| **Cites** | specs/20.8 · arch/30-core/01 §The application form · arch/30-core/03 §The eleven steps |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | container engine: docker |
| **Label** | blocking |
| **Precondition** | an image loaded into the engine from an OCI archive the test writes; a container created from it, labelled as a unit of the instance `bench-c`; a bundle whose descriptor declares a unit of kind `oneshot` naming that image, not started automatically |
| **Action** | execute the Core with `--instance bench-c`, the bundle, and `--engine` naming the engine's socket |
| **Expected** | it names the inherited container and removes it, and becomes ready. Not applicable under Docker, whose classic store loads an archive with no digest |

## yoke:the-application-invocation.07 — a Core under a guardian says it is alive once it is ready, and one with none starts all the same

| Field | Value |
| --- | --- |
| **Cites** | specs/25.55 · arch/25-guardian/06 §The signal · arch/30-core/03 §The eleven steps |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the bundle of `.02`; a listener on the account's `guardian.sock` serving the guardian's channel |
| **Action** | execute the Core; then execute it again with no `guardian.sock` |
| **Expected** | after the line saying it is ready, the listener receives one beat naming `bench-a` from the Core's process; the second becomes ready all the same |

## yoke:the-application-invocation.08 — a Core under a guardian says it is alive at its declared interval, and says when it is stopping

| Field | Value |
| --- | --- |
| **Cites** | specs/25.55 · arch/25-guardian/06 §The signal |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the bundle of `.02`, its descriptor declaring a heartbeat of `1s`; a listener on the account's `guardian.sock` |
| **Action** | execute the Core; wait three seconds; stop it with `SIGTERM` |
| **Expected** | the listener receives at least three beats before the stop, the last beat saying the Core is stopping, and the stream then ends |

## yoke:the-application-invocation.09 — a Core says whether it is confined, as the weaker arrangements in force

| Field | Value |
| --- | --- |
| **Cites** | specs/17.46 · arch/25-guardian/05 §Where a host cannot provide one |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | `yoke-core` built; a bundle; the Core executed in the application form by the test, outside any slice of its instance |
| **Action** | execute the Core, and read what it said before it was ready |
| **Expected** | it reports `confinement.no_cgroup_delegation` as a weaker finding |
