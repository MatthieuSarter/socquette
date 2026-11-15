# Socquette Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single Go binary that is an authenticated SOCKS5 (TCP + UDP) and HTTP forward proxy (`--serve`, also run as
a Windows service / systemd unit), plus a Fyne configuration UI (no argument) editing a hot-reloaded `socquette.toml`.

**Architecture:** One module, one `main` package dispatching on flags. Pure logic (config validation, auth decisions,
SOCKS5/HTTP message codecs, config diffing) lives in `internal/` packages next to thin I/O code and is unit-tested
without mocks; listeners are tested over real loopback sockets. `internal/server` owns listeners and the config poller;
`internal/control` wraps kardianos/service, UAC elevation and the background process; `internal/gui` is a thin Fyne
layer. Headless server first (Tasks 1–7), then control, UI, entry point (8–10), then CI/release and docs (11–12).

**Tech Stack:** Go 1.27 (`toolchain go1.27.4`), `fyne.io/fyne/v2` v2.8.2, `github.com/kardianos/service` v1.3.0,
`github.com/pelletier/go-toml/v2` v2.4.3, `golang.org/x/crypto` (bcrypt), `golang.org/x/sys` (Windows), `log/slog`;
tooling: golangci-lint v2.15.0, go-winres v0.3.3, repo-linter (global), python-semantic-release.

**Spec:** [docs/superpowers/specs/2026-11-15-socquette-design.md](../specs/2026-11-15-socquette-design.md) — read it
first; §x below refers to it. Oxycat (`../oxycat`) is the template for repo tooling, CI and release files.

## Global Constraints

- License **EUPL-1.2** (copy Oxycat's `LICENSE` verbatim). Module path `github.com/MatthieuSarter/socquette`.
- `go 1.27` + `toolchain go1.27.4` in `go.mod`. Max line length 120 for every style tool (golangci `lll`, yamllint,
  taplo). `golangci-lint run` and `golangci-lint fmt --diff` clean.
- File names: `socquette.toml`, `socquette.log`, `socquette.pid`, all in the config dir = `$SOCQUETTE_CONF` if
  non-empty, else the current working directory. Service name `socquette`, display name `Socquette`.
- Defaults: both proxies disabled, `listen = "0.0.0.0"`, HTTP port `8080`, SOCKS port `1080`, anonymous `false`,
  `advertise_address = ""`, no users. Config poll interval 2 s. bcrypt `DefaultCost`. Handshake (greeting/auth/request,
  HTTP request headers) timeout 30 s; no idle timeout on established tunnels; UDP association ends with its TCP conn.
- Logging (`log/slog` text handler): one line per proxied connection (`proxy`, `client`, `user` or `-`, `dst`,
  `result`) and one per auth failure; reload outcomes and bind errors. `--serve` logs to `socquette.log`, plus stderr
  when running interactively (`service.Interactive()`).
- Every user-visible string goes through `internal/i18n` (FR/EN/ES). Code, comments and docs in English.
- Coverage ≥ 80 % (aim 90 %) over `./internal/...` **excluding** `internal/gui`, `internal/control`, `internal/assets`
  and `main` (thin shells; documented in `docs/development.md`).
- Git: branch `main`; Conventional Commits with gitmoji (`feat(socks5): ✨ …`); signed commits only; **run `git commit`
  only after the owner has approved committing** (they may approve once for the whole plan); never `git push`.
- Never touch `CHANGELOG.md` (psr generates it).

## Review Focus

1. **Port change to the same port on another address** (e.g. `0.0.0.0:1080` → `127.0.0.1:1080`): bind-new-first fails
   on Windows; expect fallback close-old-then-bind, and the old listener restored if that fails too (Task 7 test
   `TestReloadSamePortNewAddress`).
2. **Config file saved mid-poll or invalid TOML**: server keeps serving with the previous config, logs one error per
   change, not one per poll (Task 7 test `TestReloadInvalidKeepsState`).
3. **Client behind NAT sending UDP from a different port than declared / port 0**: first datagram from the client IP
   binds the port; datagrams from any other IP are dropped (Task 5 tests `TestUDPLearnsPort`, `TestUDPOtherIPDropped`).
4. **Wrong credentials on an anonymous-enabled proxy**: rejected (407 / RFC 1929 failure), not downgraded (Task 3
   test `TestAnonymousWrongCredsRejected`, Task 6 test `TestHTTPAnonymousWrongCreds407`).
5. **Browser sending `Proxy-Authorization` on every request**: second check with same creds must not re-run bcrypt
   (Task 3 test `TestCheckCachesSuccess`, asserting via an injected verify-counter).

---

### Task 1: Repository scaffold and tooling

**Files:**
- Create: `go.mod`, `main.go` (temporary: prints version), `internal/version/version.go`, `LICENSE`, `.gitignore`,
  `.aiignore`, `.repolinterignore`, `.golangci.yml`, `.yamllint.yaml`, `.taplo.toml`, `justfile`,
  `justfile-custom.just`, `.just/*.just`, `.githooks/**`, `psr.toml`, `.psr/build.sh`,
  `.psr/build.sh.d/20-github-links.custom.sh`, `sonar-project.properties`

**Interfaces:**
- Produces: `version.Version` (`const Version = "0.0.0"`, stamped by psr through `version_variables =
  ["internal/version/version.go:Version", "sonar-project.properties:sonar.projectVersion"]`).

- [ ] **Step 1:** `git init -b main`; `go mod init github.com/MatthieuSarter/socquette`; set `go 1.27` /
  `toolchain go1.27.4`.
- [ ] **Step 2:** Copy from Oxycat unchanged: `LICENSE`, `.just/`, `.githooks/`, `.psr/build.sh`, `.yamllint.yaml`,
  `.taplo.toml`, `.repolinterignore`, `.aiignore`, `justfile`. Copy `.psr/build.sh.d/20-github-links.custom.sh` with
  `oxycat` → `socquette`. Copy `psr.toml` dropping `version_toml` and using the `version_variables` above. Do not copy
  `10-cargo-lock.custom.sh`.
- [ ] **Step 3:** `.gitignore` = Oxycat's repo-linter block plus `/socquette`, `/socquette.exe`, `*.syso`,
  `socquette.toml`, `socquette.log`, `socquette.pid`, `.superpowers/`.
- [ ] **Step 4:** `.golangci.yml` (v2 format): `linters.default: standard` plus `lll` (`line-length: 120`), `gosec`,
  `errorlint`, `revive`, `misspell`; `formatters: [gofmt, goimports]`.
- [ ] **Step 5:** `justfile-custom.just`: `use_global_repo_linter := "true"`; recipes `lint fix=""` (repo-linter, then
  `golangci-lint fmt` (`--diff` unless fix) and `golangci-lint run` (`--fix` if fix)), `test *args` (`go test ./...`),
  `coverage` (see Task 11 for the exact command; stub now), `run *args` (`go run . "$@"`), `build` (`go run
  github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json` on Windows, then `go build -ldflags "-H windowsgui"`
  on Windows / plain `go build` elsewhere), `icon` (`uv run --script scripts/make_icon.py`).
- [ ] **Step 6:** `sonar-project.properties`: key/name `MatthieuSarter:socquette`, `sonar.sources=.`,
  `sonar.exclusions=**/*_test.go,docs/**`, `sonar.tests=.`, `sonar.test.inclusions=**/*_test.go`,
  `sonar.go.coverage.reportPaths=.reports/coverage.out`, `sonar.coverage.exclusions` = the Global Constraints
  exclusions.
- [ ] **Step 7:** Verify: `go build ./...` succeeds; `just lint` reports nothing beyond missing docs (fixed in Task 12).
- [ ] **Step 8:** Commit `build: 🎉 scaffold the Go project` (after owner approval).

### Task 2: Configuration model, validation and file I/O (`internal/config`)

**Files:** Create `internal/config/config.go` (types, `Defaults`, `Parse`, `Marshal`, `Validate`),
`internal/config/file.go` (paths, `Load`, `Save`), tests `internal/config/config_test.go`, `file_test.go`.

**Interfaces — Produces:**
```go
type HTTP struct { Enabled bool `toml:"enabled"`; Listen string `toml:"listen"`; Port int `toml:"port"`; Anonymous bool `toml:"anonymous"` }
type SOCKS struct { Enabled bool; Listen string; Port int; Anonymous bool; AdvertiseAddress string `toml:"advertise_address"` } // same tags style
type User struct { Name string `toml:"name"`; PasswordHash string `toml:"password_hash"`; HTTP bool `toml:"http"`; SOCKS bool `toml:"socks"` }
type Config struct { HTTP HTTP `toml:"http"`; SOCKS SOCKS `toml:"socks"`; Users []User `toml:"users"` }
func Defaults() Config
func Parse(data []byte) (Config, error)          // starts from Defaults(), so missing keys keep defaults; then Validate
func Marshal(c Config) ([]byte, error)
func Validate(c Config) error                     // errors.Join of every problem, nil when valid
func (h HTTP) Addr() string; func (s SOCKS) Addr() string // net.JoinHostPort(listen, port)
type Paths struct { Dir, Config, Log, PID string }
func ResolvePaths(envConf, cwd string) Paths      // pure: envConf if non-empty else cwd; joins the three file names
func Load(path string) (cfg Config, exists bool, err error) // missing file → Defaults(), false, nil
func Save(path string, c Config) error            // Validate, Marshal, write temp file in same dir, os.Rename
```

- [ ] **Step 1: Failing tests** — `TestParseDefaultsForMissingKeys` (`[http]\nenabled=true` → HTTP.Port 8080,
  SOCKS.Port 1080, Listen `0.0.0.0`); `TestRoundTrip` (Marshal→Parse equals input with 2 users);
  `TestValidate` table: port 0 / 65536 rejected; `listen = "eth0"` rejected; `advertise_address = "my.dyndns.org"` and
  `"203.0.113.7"` accepted, `"bad host!"` rejected; HTTP and SOCKS both enabled on `0.0.0.0:1080` rejected (accepted
  when one of them is disabled); duplicate user name, empty name, 256-byte name, empty hash rejected;
  `TestValidateJoinsAllErrors` (two problems → both messages present); `TestResolvePaths` (env set / env empty);
  `TestLoadMissingFile`; `TestSaveIsAtomicAndRejectsInvalid` (invalid config → error and existing file unchanged).
- [ ] **Step 2:** Run `go test ./internal/config/` → FAIL (undefined).
- [ ] **Step 3:** Implement. Host-name check: `net.ParseIP` ok, else every dot-separated label matches
  `^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`.
- [ ] **Step 4:** `go test ./internal/config/` → PASS.
- [ ] **Step 5:** Commit `feat(config): ✨ add the TOML configuration model`.

### Task 3: Authentication (`internal/auth`)

**Files:** Create `internal/auth/auth.go`, test `internal/auth/auth_test.go`.

**Interfaces:**
- Consumes: `config.Config`, `config.User`.
- Produces:
```go
type Proto int; const ( HTTP Proto = iota; SOCKS )
func HashPassword(password string) (string, error)   // bcrypt.DefaultCost
type Authenticator struct { /* users by name, per-proto anonymous flags, success cache (sync.Map), verify func */ }
func New(c config.Config) *Authenticator
func (a *Authenticator) Anonymous(p Proto) bool
func (a *Authenticator) HasUsers(p Proto) bool       // at least one user with that right
func (a *Authenticator) Check(p Proto, name, password string) bool
```
`Check`: unknown user or user lacking the right for `p` → false; cache key = SHA-256 of `name + "\x00" + password`;
cache hit → true without bcrypt; otherwise `bcrypt.CompareHashAndPassword`, store on success. A fresh `New` per config
reload drops the cache. Keep the verify function a package-private field so tests can count calls.

- [ ] **Step 1: Failing tests** — `TestCheck` table (right user/pass/right → true; wrong pass; unknown user; user with
  `SOCKS=false` on `SOCKS` → false; same user on `HTTP` → true); `TestCheckCachesSuccess` (two calls → verify called
  once); `TestCheckFailureNotCached` (wrong then right pass → second still verified); `TestAnonymousWrongCredsRejected`
  (anonymous on, wrong creds → `Check` false: anonymity is the caller's decision only when no creds are sent);
  `TestHasUsers`.
- [ ] **Step 2:** `go test ./internal/auth/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit `feat(auth): ✨ add bcrypt user authentication with a success cache`.

### Task 4: SOCKS5 codec (`internal/socks5`, pure)

**Files:** Create `internal/socks5/codec.go`, test `internal/socks5/codec_test.go`.

**Interfaces — Produces:**
```go
const ( MethodNoAuth byte = 0x00; MethodUserPass byte = 0x02; MethodNone byte = 0xFF
        CmdConnect byte = 0x01; CmdBind byte = 0x02; CmdUDPAssociate byte = 0x03
        RepSucceeded byte = 0x00; RepGeneralFailure = 0x01; RepNotAllowed = 0x02; RepNetworkUnreachable = 0x03;
        RepHostUnreachable = 0x04; RepConnectionRefused = 0x05; RepCommandNotSupported = 0x07;
        RepAddressNotSupported = 0x08 )
type Addr struct { IP net.IP; Host string; Port uint16 } // exactly one of IP / Host set
func (a Addr) String() string                            // host:port via net.JoinHostPort
func ReadGreeting(r io.Reader) ([]byte, error)           // VER=5 check; returns offered methods
func ChooseMethod(offered []byte, anonymous, haveUsers bool) byte // prefers UserPass when haveUsers and offered
func ReadUserPass(r io.Reader) (name, password string, err error) // RFC 1929, VER=1
func WriteUserPassStatus(w io.Writer, ok bool) error     // {0x01, 0x00|0x01}
type Request struct { Cmd byte; Dst Addr }
func ReadRequest(r io.Reader) (Request, error)           // unknown ATYP → ErrAddressType
func WriteReply(w io.Writer, rep byte, bnd Addr) error   // nil/zero bnd → 0.0.0.0:0
func ParseUDP(b []byte) (frag byte, dst Addr, payload []byte, err error)
func AppendUDPHeader(dst []byte, from Addr) []byte       // RSV RSV FRAG=0 ATYP ADDR PORT
var ErrVersion, ErrAddressType error
```

- [ ] **Step 1: Failing tests** with literal byte slices: greeting `{5,2,0,2}` → `[0,2]`; `{4,…}` → `ErrVersion`;
  `ChooseMethod` table (`[0,2]` anon+users → 2; `[0]` anon → 0; `[0]` not anon → 0xFF; `[2]` anon, no users → 0xFF);
  `ReadUserPass` of `{1,5,'a','l','i','c','e',3,'p','w','d'}`; `ReadRequest` for IPv4 `{5,1,0,1,127,0,0,1,0x1F,0x90}`,
  domain `{5,1,0,3,11,"example.com",0,80}`, IPv6; ATYP 9 → `ErrAddressType`; `WriteReply` bytes for IPv4 and
  IPv6 bnd; `ParseUDP`/`AppendUDPHeader` round trip; truncated inputs → error (no panic) — `FuzzParseUDP` seed corpus
  of the above.
- [ ] **Step 2–4:** FAIL → implement → `go test ./internal/socks5/` PASS.
- [ ] **Step 5:** Commit `feat(socks5): ✨ add the SOCKS5 protocol codec`.

### Task 5: SOCKS5 server (`internal/socks5`, I/O) and the shared relay

**Files:** Create `internal/relay/relay.go` (+ `relay_test.go`), `internal/socks5/server.go`,
`internal/socks5/udp.go`, tests `internal/socks5/server_test.go`.

**Interfaces:**
- Consumes: Task 3 `*auth.Authenticator`, Task 4 codec.
- Produces:
```go
// internal/relay
func Pipe(a, b net.Conn)    // copy both ways, half-close (CloseWrite) when available, return when both done, close both
// internal/socks5
type Server struct {
    Auth      func() *auth.Authenticator // current table (atomic load by the caller)
    Advertise func() string              // current advertise_address ("" → 0.0.0.0)
    Log       *slog.Logger
    Dial      func(ctx context.Context, network, addr string) (net.Conn, error) // nil → net.Dialer
}
func (s *Server) Serve(ln net.Listener, pc net.PacketConn) error // returns when ln is closed; closes nothing it doesn't own
```
UDP design (§5.3): one goroutine reads `pc`; associations in a mutex-guarded map keyed by client IP string → list of
associations `{port uint16 (0 = unlearned), out net.PacketConn}`. Inbound datagram from `ip:port`: find an
association for `ip` with `port` equal or 0 (then learn it); none → drop. `FRAG != 0` → drop. Each association has a
dedicated `net.ListenPacket("udp", ":0")` outbound socket and a goroutine that wraps replies with `AppendUDPHeader`
and writes them to the client through `pc`. The TCP control conn handler blocks reading until EOF, then removes the
association and closes `out`. Reply BND = resolved `Advertise()` (host names via `net.DefaultResolver`, first IPv4,
resolve failure → 0.0.0.0, logged) with the SOCKS port of `pc.LocalAddr()`.

- [ ] **Step 1: Failing tests** (loopback, ports `:0`): `TestPipe` (echo both ways, closes); `TestConnectAuth` (user
  creds → reaches a local TCP echo server); `TestConnectWrongPassword` (status 0x01, conn closed);
  `TestConnectNoRight` (user with `SOCKS=false` refused); `TestConnectAnonymous`; `TestBindNotSupported` (reply 0x07);
  `TestUDPEcho` (associate, send datagram to a local UDP echo server through the relay, get it back with header);
  `TestUDPLearnsPort` (associate declaring `0.0.0.0:0`); `TestUDPOtherIPDropped` (datagram from `127.0.0.2` — skip
  the test when that address can't be bound); `TestUDPAssociationEndsWithTCP` (close control conn → later datagrams
  dropped); `TestAdvertiseAddress` (`"203.0.113.7"` appears in the reply).
- [ ] **Step 2–4:** FAIL → implement → `go test -race ./internal/socks5/ ./internal/relay/` PASS.
- [ ] **Step 5:** Commit `feat(socks5): ✨ serve SOCKS5 CONNECT and UDP ASSOCIATE`.

### Task 6: HTTP proxy (`internal/httpproxy`)

**Files:** Create `internal/httpproxy/proxy.go`, test `internal/httpproxy/proxy_test.go`.

**Interfaces:**
- Consumes: Task 3, Task 5 `relay.Pipe`.
- Produces:
```go
func ParseProxyAuth(header string) (name, password string, ok bool) // "Basic base64(name:password)", case-insensitive scheme
func RemoveHopByHop(h http.Header)  // Connection (and tokens it lists), Proxy-Connection, Keep-Alive, Proxy-Authenticate,
                                    // Proxy-Authorization, TE, Trailer, Transfer-Encoding, Upgrade
type Handler struct { Auth func() *auth.Authenticator; Log *slog.Logger; Transport http.RoundTripper /* nil → Transport{Proxy: nil} */ }
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```
Decision: header absent and `Anonymous(HTTP)` → allow as `-`; header present → must `Check`; else `407` +
`Proxy-Authenticate: Basic realm="Socquette"`. `CONNECT` → dial, `http.Hijacker`, write `HTTP/1.1 200 Connection
established\r\n\r\n`, `relay.Pipe`. Non-CONNECT with `!r.URL.IsAbs()` → `400`. Otherwise clone request, clear
`RequestURI`, `RemoveHopByHop`, `Transport.RoundTrip`, copy status/headers (minus hop-by-hop)/body; dial errors → `502`.
The caller wraps it in `http.Server{ReadHeaderTimeout: 30 * time.Second}`.

- [ ] **Step 1: Failing tests** — `TestParseProxyAuth` table (valid, lowercase `basic`, bad base64, no colon, other
  scheme); `TestRemoveHopByHop` (incl. a token listed in `Connection`); via `httptest`: `TestHTTPForward` (GET to an
  httptest origin through the proxy with creds; origin sees no `Proxy-Authorization`); `TestHTTPConnect` (tunnel to a
  TCP echo); `TestHTTPNoCreds407` (+ header value); `TestHTTPAnonymous`; `TestHTTPAnonymousWrongCreds407`;
  `TestHTTPOriginForm400`; `TestHTTPNoRight407` (user with `HTTP=false`).
- [ ] **Step 2–4:** FAIL → implement → `go test -race ./internal/httpproxy/` PASS.
- [ ] **Step 5:** Commit `feat(httpproxy): ✨ add the authenticated HTTP proxy`.

### Task 7: Server orchestration and hot reload (`internal/server`)

**Files:** Create `internal/server/server.go`, `internal/server/listeners.go`, `internal/server/logging.go`, test
`internal/server/server_test.go`.

**Interfaces:**
- Consumes: Tasks 2, 3, 5, 6.
- Produces:
```go
type Changes struct { HTTP, SOCKS bool } // listener must be (re)bound or closed
func Diff(old, new config.Config) Changes // pure: Enabled/Listen/Port differ
func NewLogger(logPath string, stderr io.Writer) (*slog.Logger, io.Closer, error) // text handler to file (append) + stderr when non-nil
func Run(ctx context.Context, paths config.Paths, log *slog.Logger, poll time.Duration) error // blocks until ctx done
```
`Run`: `Load` → apply → every `poll`, `os.Stat` the config; on (modtime, size) change → `Load`; error → log once per
distinct stamp, keep state; success → store `auth.New(cfg)` and advertise address in `atomic.Pointer`s, then for each
`Diff` flag rebind. **Rebind algorithm** (per proxy; SOCKS binds TCP and UDP as a pair): disabled → close old; enabled →
try bind new; on failure, if old exists close it and retry new; on second failure try to re-bind the old address and
log the error (proxy down only if that fails too). Missing file → `Defaults()` (both disabled), keep polling. On ctx
done close every listener.

- [ ] **Step 1: Failing tests** (temp dir, `poll = 50ms`, free ports from `net.Listen(":0")` then closed):
  `TestDiff` table; `TestRunStartsEnabledProxies`; `TestReloadUsers` (add a user to the file → new SOCKS conn with
  their creds succeeds within 1 s); `TestReloadPortChange`; `TestReloadSamePortNewAddress` (`0.0.0.0:P` →
  `127.0.0.1:P` ends listening on `127.0.0.1:P`); `TestReloadInvalidKeepsState` (garbage file → old port still serves;
  log contains the error exactly once over 5 polls); `TestDisableClosesListener`; `TestMissingFileThenCreated`;
  `TestCancelClosesAll`.
- [ ] **Step 2–4:** FAIL → implement → `go test -race ./internal/server/` PASS.
- [ ] **Step 5:** Commit `feat(server): ✨ run the proxies with hot configuration reload`.

### Task 8: Service, elevation and background process (`internal/control`)

**Files:** Create `internal/control/control.go`, `internal/control/elevate_windows.go`,
`internal/control/elevate_other.go`, `internal/control/process_windows.go`, `internal/control/process_other.go`,
test `internal/control/pid_test.go`.

**Interfaces:**
- Consumes: Task 2 `config.Paths`, Task 7 `server.Run`, `server.NewLogger`.
- Produces:
```go
type Action string; const ( Install Action = "install"; Uninstall = "uninstall"; Start = "start"; Stop = "stop" )
func ServiceConfig(confDir string) *service.Config // Name "socquette", DisplayName "Socquette", Arguments ["--serve"],
                                                   // EnvVars {"SOCQUETTE_CONF": confDir}, Option StartType "automatic"
func RunServe(paths config.Paths) error            // service.New(program, ServiceConfig(paths.Dir)).Run(); program.Start
                                                   // spawns server.Run with a cancelable ctx, Stop cancels and waits
func Do(a Action, paths config.Paths) error         // the privileged work (called by the hidden --service flag)
func Request(a Action, paths config.Paths) error    // UI side: elevated re-launch of `<exe> --service <a>`, waits, maps exit code
type State struct { Installed, Running, Background bool }
func Status(paths config.Paths) State
func StartBackground(paths config.Paths) error      // detached `<exe> --serve` with SOCQUETTE_CONF=paths.Dir, writes PID file
func StopBackground(paths config.Paths) error       // kill PID from file, remove file
func ParsePID(data []byte) (int, error)             // pure
```
Windows elevation: `windows.ShellExecuteEx` isn't in x/sys — use `ShellExecuteExW` from `shell32.dll` via
`windows.NewLazySystemDLL` with `SEE_MASK_NOCLOSEPROCESS`, verb `runas`, then `WaitForSingleObject` +
`GetExitCodeProcess`. Other OS: run the same command directly (`os.Geteuid() != 0` → error "must run as root").
Detached process on Windows: `SysProcAttr{CreationFlags: DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP, HideWindow:
true}`; Unix: `Setsid: true`. `Do(Install)` first calls `StopBackground` if a PID file exists. Liveness: Windows
`OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` + exit code `STILL_ACTIVE`; Unix `Signal(0)`.

- [ ] **Step 1:** Failing `TestParsePID` (`"1234\n"` → 1234; `""`, `"abc"`, `"-1"` → error).
- [ ] **Step 2–4:** FAIL → implement → PASS; `GOOS=linux go vet ./internal/control/` and `GOOS=windows go vet
  ./internal/control/` both clean.
- [ ] **Step 5: Manual check on Windows (admin prompt expected):** `go build -o socquette.exe .` (after Task 10) then
  `socquette.exe --service install` / `start` / `stop` / `uninstall` in an elevated shell; `sc qc socquette` shows
  `--serve`; registry `HKLM\SYSTEM\CurrentControlSet\Services\socquette\Environment` holds `SOCQUETTE_CONF=<dir>`.
- [ ] **Step 6:** Commit `feat(control): ✨ manage the service and the background server`.

### Task 9: Translations and configuration UI (`internal/i18n`, `internal/gui`, `internal/assets`)

**Files:** Create `internal/i18n/i18n.go` (+ `i18n_test.go`), `internal/assets/assets.go` (`//go:embed icon.png`),
`internal/assets/icon.png`, `scripts/make_icon.py` (PEP 723 header, pillow; draws a sock into
`internal/assets/icon.png` and `winres/icon.png`), `internal/gui/app.go`, `internal/gui/proxies.go`,
`internal/gui/users.go`, `internal/gui/theme.go`, `internal/gui/form.go` (+ `form_test.go`).

**Interfaces:**
- Consumes: Tasks 2, 3, 8.
- Produces:
```go
// internal/i18n
type Lang string; const ( FR Lang = "fr"; EN Lang = "en"; ES Lang = "es" )
type Strings struct { /* one string field per UI label, message and warning */ }
func Detect(locale string) Lang   // "fr-FR"/"fr" → FR, "es*" → ES, anything else → EN
func For(l Lang) Strings
// internal/gui
func Run(paths config.Paths) // opens the window, returns when closed
// form.go (pure, tested): edits on a config.Config copy
func AddUser(c config.Config, name, pw1, pw2 string, http, socks bool, hash func(string) (string, error)) (config.Config, error)
func SetPassword(c config.Config, name, pw1, pw2 string, hash func(string) (string, error)) (config.Config, error)
func RemoveUser(c config.Config, name string) config.Config
func ListenSuggestions(ifaceAddrs []net.Addr) []string // "0.0.0.0", "::", "127.0.0.1", then each IP, deduplicated
```
UI per §8 sketch: top bar with language `widget.Select` and theme `widget.Select` (Auto/Light/Dark, stored in
`app.Preferences()` keys `lang`, `theme`; theme via a `fyne.Theme` wrapping `theme.DefaultTheme()` that forces the
variant unless Auto); server status line + buttons driven by `control.Status` (refresh every 2 s): service installed →
`[Start|Stop]` act on the service via `control.Request`, `[Uninstall service]`; not installed → `[Start|Stop]` act on
the background process, `[Install service]`. Two proxy cards (`widget.Check`, `widget.SelectEntry` with
`ListenSuggestions(net.InterfaceAddrs())`, port `widget.Entry` with integer validation, anonymous check that asks
`dialog.NewConfirm` with the open-proxy warning before turning on); SOCKS card adds the advertise address entry. Users
list (`widget.List` or rows in a `container.VBox`): name, HTTP/SOCKS checks, change-password and delete buttons; add-user
dialog with two password entries. Bottom bar: config path, unsaved-changes label, Save (runs `config.Validate`, shows
errors in `dialog.ShowError`, else `config.Save`). Changing language rebuilds the window content.

- [ ] **Step 1: Failing tests** — `TestDetect` table; `TestAllStringsTranslated` (reflection: every field of
  `For(l)` non-empty for FR, EN, ES); `TestAddUser` (mismatched passwords → error; duplicate name → error; hash stored,
  not the password); `TestSetPassword`; `TestRemoveUser`; `TestListenSuggestions` (dedupes `127.0.0.1`).
- [ ] **Step 2–4:** FAIL → implement → `go test ./internal/i18n/ ./internal/gui/` PASS (requires a C compiler: see
  `docs/development.md`).
- [ ] **Step 5:** `just icon` produces the PNGs; commit them.
- [ ] **Step 6:** Commit `feat(gui): ✨ add the Fyne configuration window`.

### Task 10: Entry point and Windows resources (`main.go`, `winres/`)

**Files:** Modify `main.go`; create `winres/winres.json` (icon `icon.png`, manifest `execution-level: as invoker`,
`dpi-awareness: per monitor v2`, version info from the build).

**Interfaces:** Consumes Tasks 2, 7, 8, 9.

`main`: `flag` package; `--serve` → `control.RunServe(paths)`; hidden `--service <action>` → `control.Do`, exit code 0/1
(message on stderr); `--version` → print `version.Version`; any other argument → usage + exit 2; no argument →
`gui.Run(paths)`. `paths = config.ResolvePaths(os.Getenv("SOCQUETTE_CONF"), cwd)`.

- [ ] **Step 1:** Implement; `just build` produces `socquette.exe` with the icon (check in Explorer).
- [ ] **Step 2: Manual end-to-end on Windows:** run `socquette.exe` from an empty dir → window opens in the OS
  language; enable SOCKS on a port, add a user, Save → `socquette.toml` has a bcrypt hash; Start (background) →
  `curl --socks5-hostname user:pw@127.0.0.1:<port> https://example.com` works; change the port + Save → new port works
  within ~2 s, old refuses; enable HTTP → `curl -x http://user:pw@127.0.0.1:8080 https://example.com` works; Install
  service (UAC) → background process stopped, service running and serving; Stop / Uninstall.
- [ ] **Step 3:** Commit `feat: ✨ wire the serve, service and UI modes`.

### Task 11: CI, release and coverage gate

**Files:** Create `.forgejo/workflows/ci.yaml`; modify `justfile-custom.just` (`coverage`).

`coverage` recipe and CI use the same command:
```bash
pkgs=$(go list ./internal/... | grep -vE '/internal/(gui|control|assets)$' | paste -sd, -)
go test -race -coverpkg="$pkgs" -coverprofile=.reports/coverage.out $(echo "$pkgs" | tr , ' ')
go tool cover -func=.reports/coverage.out | awk '/^total:/ { sub("%","",$3); if ($3+0 < 80) { print "coverage " $3 "% < 80%"; exit 1 } else print "coverage " $3 "%" }'
```
Workflow = Oxycat's structure with Go instead of Rust (header comment: "Custom workflow: repo-linter's
forgejo-workflows rule has no Go stack."): `check-leaks` (gitleaks reusable workflow `@v13`); `check` on
`ubuntu-latest` (install Go from `actions/setup-go@v6` with `go-version-file: go.mod`; apt `gcc libgl1-mesa-dev
xorg-dev zip`; `golangci-lint` via its official install script pinned to v2.15.0; fmt diff, run, coverage command,
`sonar-reports-upload@v5`); `sonar`; `check-windows` (container `node:22-trixie`; apt `gcc-mingw-w64-x86-64 wine
wine64`; `CGO_ENABLED=1 GOOS=windows CC=x86_64-w64-mingw32-gcc go test -exec wine` on the coverage packages, `WINEDEBUG:
-all`, then `go build ./...` for windows to compile `internal/gui` and `internal/control`); `release` (psr reusable
workflow, needs the three checks); `sync` (github + gitlab); `binaries` (needs release, on new version: checkout the
tag, `go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --product-version $VERSION
--file-version $VERSION`, build `dist/socquette-${VERSION}-windows-x86_64.exe` with `CGO_ENABLED=1 GOOS=windows
CC=x86_64-w64-mingw32-gcc go build -trimpath -ldflags "-s -w -H windowsgui"` and
`dist/socquette-${VERSION}-linux-x86_64` natively, then attach with the same curl loop as Oxycat).

- [ ] **Step 1:** Write both; `just coverage` passes locally (≥ 80 %, report the number); `yamllint
  .forgejo/workflows/ci.yaml` clean.
- [ ] **Step 2:** a grep for the owner's private domain (excluding `.git/`) returns nothing.
- [ ] **Step 3:** Commit `ci: 👷 lint, test, release and attach the binaries`.

### Task 12: Documentation

**Files:** Create `README.md` (what it is, download, quick start, screenshot placeholder removed if none, license),
`AGENTS.md` (concise, agent-only: pointers to spec/plan, pure-logic rule, i18n rule, quality gate commands; no
domain), `docs/architecture.md` (packages and data flow, reload algorithm, UDP relay), `docs/configuration.md` (every
TOML key, defaults, file locations, `SOCQUETTE_CONF`), `docs/development.md` (Go toolchain auto-download, MinGW-w64
requirement for Fyne on Windows, just recipes, coverage exclusions and why), `docs/user-guide.md` (UI walkthrough,
router port forwarding TCP+UDP, advertise address/DynDNS, client setup examples for curl/Firefox, the reverse-proxy
§6 recipe, plain-text credential warning).

- [ ] **Step 1:** Write them; `just lint` fully clean.
- [ ] **Step 2:** Commit `docs: 📝 document usage, configuration and development`.
