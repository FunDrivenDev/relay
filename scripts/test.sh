#!/usr/bin/env bash
# Usage: test.sh <scope>. Runs the unit and functional tests (go test, the
# testscript files in testdata/script) when Go code or a test is in scope; the
# staged scope (pre-commit) skips the race detector to stay fast.
set -euo pipefail

scope=$1
[[ -f go.mod && -n "$(scripts/files.sh "$scope" '(\.go|^go\.(mod|sum)|^testdata/.*|^docs/.*\.md)$')" ]] || exit 0
# A test never touches the repo it runs from: a git hook exports these.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR
if [[ "$scope" == staged ]]; then
    go test ./...
else
    go test -race ./...
fi
