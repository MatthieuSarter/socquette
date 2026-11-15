#!/usr/bin/env bash

set -euo pipefail

branch="$(git symbolic-ref --quiet --short HEAD || true)"
if [ -z "$branch" ]; then
  echo "Unable to determine the current branch. Skipping origin sync check." >&2
  exit 0
fi

# A brand-new, still empty remote has no such branch yet (ls-remote exits with 2): nothing to sync with.
ls_remote_status=0
git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1 || ls_remote_status=$?
if [ "$ls_remote_status" -eq 2 ]; then
  echo "No branch '$branch' on origin yet. Skipping origin sync check." >&2
  exit 0
fi

git fetch --quiet origin "$branch" || true

remote_ref="refs/remotes/origin/$branch"
if ! git show-ref --quiet --verify "$remote_ref"; then
  echo "No origin tracking branch found for '$branch'. Skipping origin sync check." >&2
  exit 0
fi

if ! git merge-base --is-ancestor "origin/$branch" HEAD; then
  echo "Your branch '$branch' is not up to date with 'origin/$branch'." >&2
  echo "Pull/rebase the latest changes before committing." >&2
  exit 1
fi
