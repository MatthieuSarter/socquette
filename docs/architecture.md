# Architecture

One Go module, one binary. `main.go` dispatches on the command line:

| Invocation                              | What runs                                                      |
|-----------------------------------------|----------------------------------------------------------------|
| `socquette`                             | `internal/gui`: the Fyne configuration window                  |
| `socquette --serve`                     | `internal/control.RunServe` → `internal/server.Run`            |
| `socquette --service <action> --conf <dir>` | `internal/control.Do`: install/uninstall/start/stop the service (hidden, run elevated by the window) |
| `socquette --version`                   | prints `internal/version.Version`                              |

## Packages

| Package              | Role                                                                                     |
|----------------------|------------------------------------------------------------------------------------------|
| `internal/config`    | `socquette.toml` model, validation, atomic save; pure user edits used by the window      |
| `internal/auth`      | bcrypt credential checks, per-proxy rights, cache of successful checks                   |
| `internal/socks5`    | SOCKS5 codec (pure) and server: `CONNECT`, `UDP ASSOCIATE`, RFC 1929                     |
| `internal/httpproxy` | HTTP proxy handler: `CONNECT` tunnels and plain forwarding                               |
| `internal/relay`     | bidirectional TCP copy with half-close                                                   |
| `internal/server`    | owns the listeners, polls the configuration file, applies changes; logging              |
| `internal/control`   | OS service (kardianos/service), UAC elevation, background process, server status        |
| `internal/i18n`      | UI strings in French, English, Spanish                                                   |
| `internal/gui`       | Fyne window                                                                              |
| `internal/assets`    | embedded icon                                                                            |

Pure logic (validation, codecs, auth decisions, user edits) has no I/O and is unit-tested without mocks; listeners are
tested over real loopback sockets. `gui`, `control` and `main` are thin shells excluded from the coverage gate.

## Hot reload

`server.Run` calls `tick` every 2 s:

1. `os.Stat` the configuration; when its modification time or size changed, load and validate it. An invalid file is
   logged once and ignored.
2. A valid file swaps the `auth.Authenticator` and the UDP advertise address (atomic pointers read by the proxies on
   every new connection) — existing connections are untouched.
3. Each proxy's wanted address (`server.Wanted`) is compared with the address actually bound. On a difference the new
   address is bound **before** the old listener closes. If that fails on the same port (Windows refuses
   `127.0.0.1:P` while `0.0.0.0:P` is bound), the first attempt closes the old listener and retries, restoring the old
   address if the retry fails too. A failing address is retried on every tick without touching the working listener,
   so a proxy listening on an interface IP that isn't up yet at boot starts as soon as it is.

Closing a listener stops new clients only; HTTP keep-alive connections of the old listener end after 2 minutes idle.

## SOCKS5 UDP relay

Built for a server behind a home router (a single forwarded port):

- One UDP socket on the SOCKS listen address and port number, shared by every association.
- `UDP ASSOCIATE` registers an association for the client's IP and answers with the advertise address and that port.
  The client's UDP port is learned from its first datagram (the declared one is ignored: a NAT rewrites it); datagrams
  from other addresses are dropped, as are fragmented ones.
- Each association has its own outbound UDP socket, so replies map back to the right client; it lives as long as the
  client's TCP control connection.

## Service and elevation

The window runs unelevated. Service actions re-run the binary through `ShellExecuteEx` with the `runas` verb (UAC
prompt) as `--service <action> --conf <dir>`, since an elevated process doesn't inherit the caller's environment.
The service definition sets `SOCQUETTE_CONF` so the service finds the configuration whatever its working directory.
Service status is read with query-only rights, which every user has. Without the service, the window starts a
detached `--serve` process and tracks it through `socquette.pid`.

The Windows build is a GUI program (no console window on double-click); with arguments, it attaches to the parent
console so `--serve` and `--version` print there.
