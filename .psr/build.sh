#!/usr/bin/env bash
# Run by python-semantic-release as its build_command, after it has stamped the new version into the
# version files and before it creates the release commit. Runs every script under .psr/build.sh.d/,
# in name order, and aborts the release on the first failure. Each script stages what it changes
# with `git add`, so that lands in the release commit.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -d .psr/build.sh.d ]; then
  echo "No .psr/build.sh.d directory found. Skipping."
  exit 0
fi

while IFS= read -r script; do
  echo "Running PSR build script: $script"
  /usr/bin/env bash "$script"
done < <(find .psr/build.sh.d -type f -name "*.sh" | sort)
