# Socquette — design

Date: 2026-11-15 · Status: draft, awaiting approval

Notation: **(owner)** = decided by the owner; **(Claude's call)** = a default chosen during brainstorming that the owner
can still override; **(builder's call)** = left open on purpose.

## 1. Intent

**Socquette** is a small authenticated forward proxy (SOCKS5 + HTTP) that runs on a Windows PC at home, reachable from
the Internet through the home router's port forwarding, plus a desktop configuration UI so the proxy can be set up
without hand-editing files. Linux support is a nice to have that the design must not rule out. The project is public,
under **EUPL-1.2** **(owner)**.

## 2. Goals

- One single binary **(owner)**:
  - `socquette --serve`: server / daemon mode (also what the OS service runs).
  - `socquette` with no argument: opens the configuration UI.
- SOCKS5 proxy, with `CONNECT` and `UDP ASSOCIATE` (UDP is needed: "as much traffic as possible") **(owner)**.
- HTTP proxy: `CONNECT` tunnels (HTTPS) and plain HTTP forwarding **(Claude's call, accepted)**.
- Configuration in a TOML file, hot-reloaded by the running server when the UI (or anyone) changes it **(owner)**.
- The UI edits: HTTP proxy on/off, listen address, port, anonymous access; same for SOCKS (plus the UDP advertised
  address, §5.3); a list of users with a password and, per user, an HTTP right and a SOCKS right **(owner)**; a list
  of trusted networks (IPs / CIDR subnets accepted without credentials), with the same rights **(owner, issue #1)**.
- The UI installs/uninstalls the server as a Windows service, and starts/stops the server — the service when installed,
  a background process otherwise **(owner)**.
- Can sit behind a TLS-terminating TCP stream (e.g. a Nginx Proxy Manager "Stream") for the HTTP proxy with no change to
  Socquette **(owner's question; nothing to build, see §6)**.

## 3. Non-goals and anti-goals

**Non-goals:**

- TLS on Socquette's own listeners (owner chose plain-text auth for v1; TLS can come from a reverse proxy for HTTP).
- SOCKS5 `BIND`, SOCKS4/4a, GSSAPI auth, PROXY protocol, per-user bandwidth limits, fail2ban-style banning
  **(Claude's call, YAGNI)**. (IP allow-lists were one too, until issue #1: see trusted networks, §5.1.)
- Trusting `X-Forwarded-For` or the PROXY protocol for the client address: a trusted network matches the TCP peer.
- Interrupting open connections when a user is removed or loses a right: changes apply to new connections only
  **(Claude's call, accepted)**.
- Log rotation (the log file just grows; ponytail note, add rotation if it ever matters) **(Claude's call)**.

**Anti-goals — it fails even if it "works" when:**

- Its default configuration is an open proxy. Anonymous access is opt-in per proxy, off by default, and the UI warns
  when it is turned on.
- Passwords are stored in clear in the TOML file.
- A config edit takes the proxy down: a bad file or a port that can't be bound keeps the previous working state.
- The release depends on any private infrastructure (private Nexus, private registries) or the repository mentions a
  private domain of the owner anywhere, including agent instruction files **(owner)**.

## 4. Configuration file

### 4.1 Location **(owner)**

`socquette.toml` in the directory named by the `SOCQUETTE_CONF` environment variable when set, else in the current
working directory. The log file `socquette.log` and the background-process PID file `socquette.pid` live in the same
directory **(Claude's call)**.

A service's working directory is `C:\Windows\System32`, so when the UI installs the service, it records the absolute
config directory in the service definition, so the service reads the very file the UI edits. Mechanism **(builder's
call)**: `SOCQUETTE_CONF` in the service environment (kardianos/service `EnvVars`) or an extra hidden argument; verify
on both Windows and systemd.

### 4.2 Format **(Claude's call)**

```toml
[http]
enabled = true
listen = "0.0.0.0"     # IP address of the interface to listen on ("0.0.0.0" / "::" = all)
port = 8080
anonymous = false      # allow unauthenticated clients

[socks]
enabled = true
listen = "0.0.0.0"
port = 1080            # TCP and UDP (the UDP relay uses the same port number)
anonymous = false
advertise_address = "" # IP or host name announced for UDP; empty = 0.0.0.0 (clients reuse the server address)

[[users]]
name = "alice"
password_hash = "$2a$10$..." # bcrypt
http = true
socks = true

[[networks]]                 # trusted network: clients from these addresses need no credentials
name = "lan"                 # shown in the log's user column
addresses = ["192.168.1.0/24", "fd00::/8", "203.0.113.7"] # bare IP = /32 or /128
http = true
socks = true
```

- Missing file: the server runs with both proxies disabled and keeps polling; the UI starts from defaults (both
  proxies disabled, ports 8080 / 1080, listen `0.0.0.0`, no users) and creates the file on first save.
- Validation (pure function): ports 1–65535; `listen` must parse as an IP address; `advertise_address` is empty, an
  IP address or a host name;
  HTTP and SOCKS can't share the same listen address + port; user names unique and non-empty; name and password at
  most 255 bytes (RFC 1929 limit); no user with an empty hash; network names unique and non-empty, at least one
  address per network, each an IP or a CIDR subnet (`netip.ParseAddr` / `netip.ParsePrefix`).
- The UI writes the file atomically (temp file in the same directory + rename) so the server never reads a half-written
  file.

### 4.3 Hot reload **(Claude's call, accepted)**

The server checks the file's modification time and size every 2 seconds (stdlib only; robust to editors that replace
the file atomically, which breaks per-file fsnotify watches). On change:

- Parse + validate. On error: log it, keep the current state.
- Swap the user table atomically (`atomic.Pointer`): new connections see the new users/rights right away.
- For each proxy whose `enabled` / `listen` / `port` changed: bind the new listener first, then close the old one. If
  the bind fails, log it and keep the old listener. Disabling closes the listener. Open connections are never cut.

## 5. Proxies

### 5.1 Authentication (shared)

- Users are checked with bcrypt. Browsers re-send credentials on every HTTP proxy request, so successful checks are
  cached in memory, keyed by a SHA-256 of `name:password`, and the cache is dropped on every config reload
  **(Claude's call)**.
- A proxy with `anonymous = true` also accepts unauthenticated clients; authenticated clients still work and are logged
  under their name; wrong credentials are still rejected, not downgraded to anonymous **(Claude's call)**.
- A user without the right for a proxy is rejected by that proxy like a wrong password.
- **Trusted networks** (issue #1) **(owner: named entries; wrong credentials rejected)**: a client that sends no
  credentials is accepted when its TCP source address (unmapped from `::ffff:a.b.c.d`, zone dropped) is in a network
  with the right for that proxy, logged under the first such network's name; else it falls back to `anonymous`. A
  client that sends credentials is judged on them only, so wrong credentials are rejected even from a trusted network.
  The decision is `auth.Authenticator.Unauthenticated`, a pure function over the parsed prefixes.
- Log line per connection: time, proxy, client address, user (network name, or `-` when anonymous), destination, result; and every auth failure
  **(Claude's call)**. Log to `socquette.log` and to stderr.

### 5.2 HTTP proxy

- `Proxy-Authorization: Basic …`; bad credentials, or missing ones outside a trusted network on a non-anonymous proxy
  → `407` with `Proxy-Authenticate: Basic realm="Socquette"`.
- `CONNECT host:port`: hijack the client connection, dial the target, answer `200`, then copy both ways.
- Other methods (absolute-form URL): forward with an `http.Transport` that has **no** upstream proxy (never read
  `HTTP_PROXY` from the environment), stripping hop-by-hop headers and `Proxy-Authorization`.
- Requests that aren't proxy requests (origin-form URL, i.e. someone browsing the proxy port directly) → `400`.

### 5.3 SOCKS5 proxy (RFC 1928 + RFC 1929)

- Methods: `0x02` (username/password) always offered when users exist; `0x00` (no auth) only when `anonymous = true`
  or the client's address is in a trusted network with the SOCKS right. No acceptable method → `0xFF` and close.
- Commands: `CONNECT` (`0x01`); `UDP ASSOCIATE` (`0x03`); `BIND` → reply `0x07` (command not supported).
- Address types: IPv4, IPv6, domain name (resolved by the server).
- **UDP relay**, built for a server behind a home NAT **(Claude's call, accepted)**:
  - One UDP socket bound to the same `listen` address and port number as the SOCKS TCP listener, so a single TCP+UDP
    port forwarding rule on the router is enough.
  - The `UDP ASSOCIATE` reply announces `advertise_address` (the public IP, or a host name such as a DynDNS one,
    resolved to an IP at each `UDP ASSOCIATE` since home IPs change) and the SOCKS port; empty → `0.0.0.0`, which
    most clients read as "the address you connected to".
  - An association belongs to the authenticated TCP control connection that created it and dies with it. Datagrams
    are accepted only from that client's IP (so a trusted network's client authorizes its own datagrams only, through
    its TCP control connection); the client's UDP source port is learned from its first datagram when the
    request said port 0.
  - Each association gets its own outbound UDP socket, so replies from targets map back to the right client; replies
    are wrapped in the SOCKS UDP header and sent to the client from the relay socket.
  - Fragmented datagrams (`FRAG != 0`) are dropped (allowed by the RFC).

## 6. Behind a reverse proxy (no code)

Documented in the user guide: for the HTTP proxy, a TLS-terminating TCP stream (Nginx Proxy Manager "Streams", nginx
`stream` module, stunnel) in front of Socquette lets clients use it as an "HTTPS proxy" (PAC `HTTPS host:port`, `curl
--proxy https://…`), encrypting the proxy credentials. An HTTP "Proxy Host" won't work (nginx's HTTP module doesn't do
`CONNECT`). Socquette then sees the reverse proxy's IP as the client address. Not applicable to SOCKS5 nor UDP.

## 7. Server control and the Windows service

- Service management through `github.com/kardianos/service` (Windows service + systemd on Linux) **(owner: approach
  A)**. Service name `socquette`, runs `socquette --serve`, automatic start, LocalSystem (root on Linux).
- Installing, uninstalling, starting and stopping a service need admin rights. The UI itself runs unelevated
  (`asInvoker` manifest); each of these actions re-launches the binary elevated (UAC prompt via `ShellExecute` verb
  `runas`) with a hidden action flag, e.g. `socquette --service install|uninstall|start|stop`, and waits for it
  **(Claude's call)**. On Linux, the same hidden action runs directly and reports an error when not root
  **(Claude's call)**.
- Without the service, "Start" launches a detached `socquette --serve` process (PID written to `socquette.pid`) that
  outlives the UI; "Stop" kills it **(Claude's call)**. Installing the service first stops such a background process
  (they would fight over the ports).
- The UI shows the server state: service installed or not; running or stopped (service status, or PID file + live
  process check).
- Windows build uses the GUI subsystem (`-H windowsgui`) so double-clicking doesn't open a console window; `--serve`
  then logs to the file only when no console is attached.

## 8. Configuration UI

Fyne (`fyne.io/fyne/v2`) **(owner)**, which requires CGO (MinGW-w64 on Windows; `gcc-mingw-w64` + OpenGL/X11 headers in
CI). Layout sketch **(Claude's call)**:

```
┌ Socquette ────────────────────────────────── [Language ▾] [Theme ▾] ┐
│ Server: ● running (service)     [Stop]  [Uninstall service]         │
├─ HTTP proxy ──────────────────────┬─ SOCKS5 proxy ──────────────────┤
│ [x] Enabled                       │ [x] Enabled                     │
│ Listen [0.0.0.0      ▾] Port [8080]│ Listen [0.0.0.0     ▾] Port[1080]│
│ [ ] Anonymous access  ⚠           │ [ ] Anonymous access  ⚠         │
│                                   │ Public address (UDP) [        ] │
├─ Users ─────────────────────────────────────────────────────────────┤
│ Name        HTTP   SOCKS                                            │
│ alice       [x]    [x]     [Change password]  [Delete]              │
│ [+ Add user]                                                        │
├─ Trusted networks ──────────────────────────────────────────────────┤
│ Name   Addresses             HTTP  SOCKS                            │
│ lan    192.168.1.0/24, …     [x]   [x]   [Edit addresses] [Delete]  │
│ [+ Add network]                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ C:\…\socquette.toml                        (unsaved changes) [Save] │
└─────────────────────────────────────────────────────────────────────┘
```

- Listen fields are editable dropdowns suggesting `0.0.0.0`, `::`, `127.0.0.1` and the machine's own interface
  addresses.
- Explicit **Save** with an unsaved-changes marker; validation errors shown before writing **(Claude's call)**. The UI
  reads the file when it opens and doesn't watch it afterwards (one editor at a time is the expected use).
- Passwords are entered twice when added/changed, hashed by the UI, never shown again.
- Turning anonymous access on shows a confirmation warning (open proxy on the Internet).
- Languages FR/EN/ES, initial one from the OS locale, selector in the window; themes light/dark/auto (auto follows the
  OS), selector in the window, default auto. Both preferences stored with Fyne's preferences API, not in the TOML.

## 9. Project conventions (from the owner's global rules)

- Go module path `github.com/MatthieuSarter/socquette` (the public mirror) **(Claude's call)**; latest GA Go.
- Code, comments and project docs in English; UI strings in FR/EN/ES.
- Pure logic (config parsing/validation, SOCKS/HTTP message parsing and encoding, auth decisions) kept free of I/O and
  unit-tested; I/O shells (listeners, file polling, service, GUI) stay thin. Coverage gate 80%, target 90%, excluding
  the GUI, platform/service and `main` shells (like Oxycat).
- Docs: README, AGENTS.md (concise, agent-only), `docs/architecture.md`, `docs/development.md`,
  `docs/configuration.md`, `docs/user-guide.md`; LICENSE = EUPL-1.2 text.
- justfile (`lint`, `test`, `coverage`, `run`, `build`), repo-linter with `use_global_repo_linter := "true"` (not a
  Python project), `.reports/` git-ignored, line length 120 for every style tool, default branch `main`.
- Versioning by python-semantic-release from Conventional Commits; changelog commit links rewritten to the public
  GitHub repository (`.psr/build.sh.d/20-github-links.custom.sh`, as in Oxycat). The version lives in a Go source
  constant stamped by psr.
- CI: custom `.forgejo/workflows/ci.yaml` (repo-linter has no Go stack), modelled on Oxycat's: gitleaks, lint + tests +
  coverage, Sonar, release, GitHub/GitLab sync, then cross-build the Windows binary (and the Linux one) and attach them
  to the release. Only public actions, images and Go modules; the Forgejo server URL comes from the runner context,
  never written in the repository.

## 10. Left to the builder

Package layout; TOML library (`pelletier/go-toml/v2` suggested); icon (a sock fits the name); Windows resources
(icon + `asInvoker` manifest, e.g. via `go-winres`); exact log format; idle timeouts on tunnels and UDP associations;
the mechanism of §4.1; whether Windows tests also run under Wine in CI.
