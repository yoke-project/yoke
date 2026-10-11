# yoke

## What it is

The Core of Yoke and everything that has to change with it: the contract definitions, the Core
itself, the conformance suite that holds every language library to the contract, and the verification
tool that checks this project's own tests. The Core supervises, routes and persists on a single host,
and never interprets what a plugin's payload means.

It is not a library to write plugins with: those are the SDK families, one repository per language,
which take their definitions from here.

## What is where

| Path | What it holds |
| --- | --- |
| `proto/` | The contract definitions, a Go module of its own (`github.com/yoke-project/yoke/proto`), tagged `proto/vX.Y.Z`. The same definitions are published as the `yoke-proto` crate and wheel; their sources are generated under `proto/rust/` and `proto/python/` |
| `cmd/yoke-core/` | The Core, one process per instance |
| `cmd/yoke-conformance/` | The conformance suite, published as a binary per platform |
| `cmd/yoke-verify/` | The verification tool, published as a binary per platform |
| `cmd/yoke-release/` | This repository's release verb |
| `internal/core/` | The Core's parts: supervisor, registry, discovery, event bus, log store, the surfaces |
| `internal/gate/` | The gate every description passes before it is used |
| `internal/conformance/` | The suite's cases |
| `releases/manifest.jsonl` | One line per publication, for every release of the project |
| `checks/`, `ci/` | This repository's own checks, and the scripts continuous integration runs |

A test's description sits beside the test, as a `.std.md` file.

## Working on it

Go, [`just`](https://just.systems) 1.58.0 or newer, and a container engine (Podman or Docker): the
crate's and the wheel's sources are generated in pinned images, and the Core's container tests drive
the engine. The `Containerfile` builds a container with the toolchain, for building without
installing Go.

| Command | What it does |
| --- | --- |
| `just build` | Builds both modules, and checks the definitions |
| `just test` | Runs the checks, every test of both modules, and the conformance suite against the Go family |
| `just lint` | Parses every shell script and runs `go vet` |
| `just fmt` | Fails, naming each file, when a Go file is not formatted |
| `just develop` | Verifies the installed `just` against a floor, given as `just develop 1.58.0` |
| `just release` | Publishes what the tags on this commit name; run by the release, not by hand |

## Where the depth is

The documentation at [docs.yoke-project.dev](https://docs.yoke-project.dev/): the
[reference](https://docs.yoke-project.dev/reference/) for the contracts and the Core's behaviour, and
the [contributors' section](https://docs.yoke-project.dev/contributing/) for how the work is done.

## Licence

Apache 2.0, in [`LICENSE`](LICENSE), with a [`NOTICE`](NOTICE).
