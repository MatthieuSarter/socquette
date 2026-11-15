# Display the list of available just commands
@_help:
    just --list

[unix]
set shell := ["bash", "-uc"]

set allow-duplicate-recipes := true
set allow-duplicate-variables := true

import ".just/main.just"
import? "justfile-custom.just"
import? "../justfile"
