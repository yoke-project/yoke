# A Plugin in a container

| | |
| --- | --- |
| **Feature** | a unit of kind `plugin` declared with an image in a descriptor: launched by the application form's Core on the engine, reaching the Core through the instance's directory mounted at its own path, and registering, holding its Session and stopping exactly as it would on the host |
| **Planning item** | yoke-project/yoke#235 |

## yoke:a-plugin-in-a-container.01 — a Plugin written for the host registers from a container, unchanged

| Field | Value |
| --- | --- |
| **Cites** | specs/45.3 · specs/45.5 · specs/45.7 · specs/45.8 · arch/35-units/05 §What containerisation is allowed to change · arch/35-units/05 §The channel across the boundary · arch/35-units/05 §Identity across the boundary |
| **Level** | L3 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | the environment's engine serving its API; an image holding a Plugin built against the Go library, unchanged from the one a host would run; a bundle whose descriptor declares a unit `bench` of kind `plugin` naming that image, and whose `manifests/` holds the Plugin's Manifest |
| **Action** | execute the Core in the application form with `--engine`; wait; stop it with `SIGTERM` |
| **Expected** | the unit reaches `Running`, admitted and registered, from inside a container labelled with the instance; on the stop the unit ends and the Core exits zero, leaving no container of the instance running |
