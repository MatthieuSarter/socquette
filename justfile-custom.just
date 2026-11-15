# Go project: no project-local Python environment, so use the globally installed repo-linter.
use_global_repo_linter := "true"

# Packages with tests: everything but the GUI and main, which need CGO (a C compiler) to build and have no tests.
test_pkgs := "$(go list -e ./internal/... | grep -vE '/internal/gui$' | paste -sd' ' -)"
# Packages gated by the coverage threshold: the tested ones but the service/platform and assets shells.
coverage_pkgs := "$(go list -e ./internal/... | grep -vE '/internal/(gui|control|assets)$' | paste -sd, -)"

# Run repo-linter, then the Go format and lint checks
[group("Misc")]
lint fix="" extra_opts=lint_extra_opts:
    just _repo_linter lint {{ fix }} {{ extra_opts }}
    just check-format {{ fix }}
    just check {{ fix }}

# Run golangci-lint (with --fix: apply the automatic fixes)
[group("Quality")]
check fix="":
    golangci-lint run {{ if fix != "" { "--fix" } else { "" } }}

# Check (or apply, with --fix) the code format
[group("Quality")]
check-format fix="":
    golangci-lint fmt {{ if fix != "" { "" } else { "--diff" } }}

# Run the test suite (no C compiler needed)
[group("Testing")]
[positional-arguments]
test *args:
    go test {{ test_pkgs }} "$@"

# Coverage report of the gated packages (80% minimum), written to .reports/coverage.out
[group("Testing")]
coverage:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p .reports
    pkgs="{{ coverage_pkgs }}"
    go test -coverpkg="$pkgs" -coverprofile=.reports/coverage.out $(echo "$pkgs" | tr , ' ')
    go tool cover -func=.reports/coverage.out | awk '/^total:/ { sub("%", "", $3); \
        if ($3 + 0 < 80) { print "coverage " $3 "% < 80%"; exit 1 } else print "coverage " $3 "%" }'

# Run socquette with optional arguments
[positional-arguments]
run *args:
    go run . "$@"

# Build the binary (on Windows: with its icon and manifest, without a console window)
[windows]
build:
    go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64
    go build -trimpath -ldflags "-H windowsgui" -o socquette.exe .

# Build the binary
[unix]
build:
    go build -trimpath -o socquette .

# Redraw the icon assets (internal/assets/icon.png and winres/icon.png)
icon:
    uv run --script scripts/make_icon.py
