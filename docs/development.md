# Development

## Requirements

- Go: `go.mod` pins the toolchain; an older local Go downloads it automatically.
- [just](https://just.systems), [golangci-lint](https://golangci-lint.run) v2, repo-linter (installed globally).
- For the configuration window (Fyne, which uses CGO) a C compiler:
  - Windows: MinGW-w64, e.g. `winget install BrechtSanders.WinLibs.POSIX.UCRT`, then `gcc` must be in the `PATH`.
  - Linux: `gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev`.

  Everything but `internal/gui` and `main` builds and tests without it, but `just lint` (golangci-lint type-checks
  the GUI) and `just build` need it. Docker works too: see below.

## Recipes

| Recipe          | What it does                                                                   |
|-----------------|--------------------------------------------------------------------------------|
| `just lint`     | repo-linter, `golangci-lint fmt --diff`, `golangci-lint run` (`--fix` to fix)  |
| `just test`     | tests of every package but the GUI (no C compiler needed)                      |
| `just coverage` | coverage gate (80% minimum, aim 90%), report in `.reports/coverage.out`        |
| `just run`      | `go run .` with the given arguments                                            |
| `just build`    | the binary; on Windows with icon and manifest, no console window (needs a C compiler) |
| `just icon`     | redraws `internal/assets/icon.png` and `winres/icon.png`                       |

The coverage gate excludes `internal/gui`, `internal/control`, `internal/assets` and `main`: thin shells over Fyne,
the service manager and the OS, checked by hand. The race detector needs CGO: CI runs the tests with `-race`.

### Cross-compiling the Windows build from Linux (or Docker)

```bash
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64
CGO_ENABLED=1 GOOS=windows CC=x86_64-w64-mingw32-gcc go build -trimpath -ldflags "-H windowsgui" -o socquette.exe .
```

`go-winres` writes `rsrc_windows_amd64.syso` (icon, `asInvoker` manifest, version info), picked up by `go build`.

## CI and releases

`.forgejo/workflows/ci.yaml` is a custom workflow (repo-linter has no Go stack): gitleaks, lint + tests + coverage
(Sonar), Windows build and tests under Wine, then python-semantic-release creates the version from the Conventional
Commits, the repository is mirrored to GitHub and GitLab, and the Windows and Linux binaries are built and attached to
the release. The version lives in `internal/version/version.go`, stamped by psr. `.psr/build.sh.d/` rewrites the
changelog's commit links to the public GitHub mirror.

Only public Go modules, actions and container images are used.
