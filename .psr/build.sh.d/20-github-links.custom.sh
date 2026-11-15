#!/usr/bin/env bash
# Point the changelog's commit links, which python-semantic-release builds from the private Forgejo remote, to the
# public GitHub repository instead (same commit hashes, since it's a mirror).
set -euo pipefail
sed -i 's#https\?://[^/)]*/MatthieuSarter/socquette/commit/#https://github.com/MatthieuSarter/socquette/commit/#g' CHANGELOG.md
git add CHANGELOG.md
