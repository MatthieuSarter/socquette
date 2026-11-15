#!/usr/bin/env bash

if [[ -f poetry.lock ]]; then
  poetry install
  poetry run repo-linter lint
else
  repo-linter lint
fi
