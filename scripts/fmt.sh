#!/usr/bin/env bash
# Usage: fmt.sh <scope>. Formats the files in scope, fixing what the formatters can.
set -euo pipefail

scope=$1
in_scope() { scripts/files.sh "$scope" "$1"; }

if [[ -n "$(in_scope '\.go$')" ]]; then
    golangci-lint fmt
    in_scope '\.go$' | xargs -r gofumpt -w
fi
in_scope '\.md$' | xargs -r rumdl fmt --quiet
in_scope '(^scripts/.*|^install)\.sh$' | xargs -r shfmt -w
in_scope '\.toml$' | xargs -r env RUST_LOG=warn taplo fmt --colors never
in_scope '\.ya?ml$' | xargs -r yamlfmt
if [[ -n "$(in_scope '^Justfile$')" ]]; then just --unstable --fmt; fi
