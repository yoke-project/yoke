# The graphical session

| | |
| --- | --- |
| **Feature** | what a unit needing a display or audio is given when its instance was started before the graphical session: each need is expanded at every launch from the Core's environment, which is the environment of the guardian that started the Core and is fixed for the Core's life, and from the host's filesystem as it is at that launch; so a display named in that environment is found as soon as its socket exists, a display it does not name is never found, and audio, found by its default sockets under the runtime directory, is found as soon as they exist; and a guardian already running carries its own environment to every Core it starts, whatever the session that asks for an instance holds |
| **Planning item** | yoke-project/yoke#460 |

## yoke:the-graphical-session.01 — a socket that appears after the Core started is found only where the Core's environment named it

| Field | Value |
| --- | --- |
| **Cites** | specs/45.16 · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a Core whose environment names a runtime directory and no display, as a guardian started before any compositor has; a second whose environment names `WAYLAND_DISPLAY=wayland-1` under that directory, with no socket there yet; units on the host needing `display`, and one needing `audio` |
| **Action** | under each, launch the units; then create the Wayland socket and PipeWire's under the runtime directory, as a compositor and a session's audio do when they start; and launch them again |
| **Expected** | before the sockets exist every unit is `Failed`, its failure naming its need; after, under the first Core the display is still `Failed` and audio is handed `PIPEWIRE_REMOTE`; under the second the display is handed `WAYLAND_DISPLAY` as the socket's absolute path |

## yoke:the-graphical-session.02 — a guardian already running starts a Core with its own environment, not the session's

| Field | Value |
| --- | --- |
| **Cites** | specs/17.33 · specs/17.34 · specs/45.16 · arch/35-units/05 §What each declared need expands into |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | an account with no guardian running; the manager invoked from a session that names no display, as a login before the compositor or the account's systemd at boot is |
| **Action** | start one instance from that session, which starts a guardian that the running instance keeps; then, from a session naming `WAYLAND_DISPLAY`, `DISPLAY` and `XAUTHORITY`, start a second instance |
| **Expected** | the second instance's Core is started by the guardian already running, and with none of the second session's display variables |
