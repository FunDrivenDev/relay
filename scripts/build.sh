#!/usr/bin/env bash
# Usage: build.sh. Builds $BIN (exported by the Justfile), its version from git describe.
set -euo pipefail

version=$(git describe --tags --always --dirty)
go build -trimpath -ldflags "-s -w -X main.version=$version" -o "${BIN:-bin/relay}" ./cmd/relay
echo "built ${BIN:-bin/relay} ($version)" >&2
