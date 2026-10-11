#!/usr/bin/env bash
# Usage: lint.sh <scope> [family]. Checks the files in scope without changing
# them; `family` narrows it to go, specs, markdown, spelling, agents, shell,
# just, toml, yaml or release.
set -euo pipefail

scope=$1
family=${2:-}
want() { [[ -z "$family" || "$family" == "$1" ]]; }
in_scope() { scripts/files.sh "$scope" "$1"; }
go_in_scope() { [[ -f go.mod && -n "$(in_scope '(\.go|^go\.(mod|sum)|^testdata/.*|^\.golangci\.yml)$')" ]]; }

if want go && go_in_scope; then
    unformatted=$(in_scope '\.go$' | xargs -r gofumpt -l)
    if [[ -n "$unformatted" ]]; then
        echo "not gofumpt-formatted (just fmt): $unformatted" >&2
        exit 1
    fi
    go mod tidy -diff
    go vet ./...
    golangci-lint run
fi
# Every command has a summary, help and a JSON input spec; every entity a docs page.
if want specs && [[ -f go.mod && -n "$(in_scope '(\.go|^docs/.*\.md)$')" ]]; then
    go test -count=1 -run '^TestRegistry' ./...
fi
if want markdown; then in_scope '\.md$' | xargs -r rumdl check --quiet; fi
if want spelling; then in_scope '.' | xargs -r typos --force-exclude; fi
# Agents read AGENTS.md; a CLAUDE.md would hide it from Claude Code.
if want agents; then
    found=$(in_scope '(^|/)CLAUDE(\.local)?\.md$')
    if [[ -n "$found" ]]; then
        echo "use AGENTS.md instead of: $found" >&2
        exit 1
    fi
fi
if want shell; then
    in_scope '(^scripts/.*|^install)\.sh$' | xargs -r shellcheck
    in_scope '(^scripts/.*|^install)\.sh$' | xargs -r shfmt -d
fi
if want just && [[ -n "$(in_scope '^Justfile$')" ]]; then just --unstable --fmt --check; fi
if want toml; then
    in_scope '\.toml$' | xargs -r env RUST_LOG=warn taplo fmt --check --colors never
    in_scope '\.toml$' | xargs -r env RUST_LOG=warn taplo lint --colors never
fi
if want yaml; then in_scope '\.ya?ml$' | xargs -r yamlfmt -lint; fi
if want release && [[ -n "$(in_scope '^\.goreleaser\.yaml$')" ]]; then goreleaser check --quiet; fi
