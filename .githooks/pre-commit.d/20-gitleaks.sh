#!/usr/bin/env bash

set -euo pipefail

if ! command -v gitleaks &> /dev/null; then
  echo "gitleaks is not installed. Install it from https://github.com/gitleaks/gitleaks and try again." >&2
  exit 1
fi

gitleaks git --pre-commit --staged --redact --verbose
