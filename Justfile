# The only entry point for building and checking relay; every tool comes pinned
# from mise.toml.
#
# A scope picks the files a recipe looks at:
#   all      every tracked and untracked file (the default)
#   changed  files that differ from origin/main, committed or not; everything
#            when the toolchain itself changed
#   staged   files in the git index

set shell := ["bash", "-euo", "pipefail", "-c"]

# The binary `just build` writes.

export BIN := "bin/relay"

# List the recipes.
default:
    @just --list --unsorted

# Once per clone: install the pinned tools and the Go modules.
init:
    mise install --locked
    go mod download

# Format the files in scope, fixing what the formatters can.
fmt scope="all":
    @scripts/fmt.sh {{ quote(scope) }}

# Lint the files in scope; `family` narrows it to go, specs, markdown, spelling, agents, shell, just, toml, yaml or release.
lint scope="all" family="":
    @scripts/lint.sh {{ quote(scope) }} {{ quote(family) }}

# Run the unit and functional tests when Go code or a test is in scope (staged: without the race detector).
test scope="all":
    @scripts/test.sh {{ quote(scope) }}

# Lint, then test.
check scope="all": (lint scope) (test scope)

# Audit for secrets and personal data in the whole history and vulnerable Go modules.
audit:
    @scripts/audit.sh

# Build the binary into bin/relay, its version from git describe.
build:
    @scripts/build.sh

# Build the release archives and packages into dist/ (snapshot), or release origin/main (publish [--dry-run] [vX.Y.Z]): the version computed from the commits since the last tag (`toolbox release next`), or an existing tag to finish, tagged and published by goreleaser, nothing committed.
release mode="snapshot" *args:
    @scripts/release.sh {{ quote(mode) }} {{ args }}

# Run relay from the sources: just run session list --json.
run *args:
    go run ./cmd/relay {{ args }}
